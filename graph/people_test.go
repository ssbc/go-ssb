// SPDX-License-Identifier: MIT

package graph

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
)

type PeopleOp interface {
	Op(*testState) error
}

type testState struct {
	t         *testing.T
	peers     map[string]*publisher
	refToName map[string]string
	store     testStore
}

type PeopleOpNewPeer struct{ name string }

func (op PeopleOpNewPeer) Op(state *testState) error {
	var opk PeopleOpNewPeerWithAglo
	opk.name = op.name
	opk.algo = ssb.RefAlgoFeedSSB1
	return opk.Op(state)
}

type PeopleOpNewPeerWithAglo struct {
	name string
	algo string
}

var i uint64

func (op PeopleOpNewPeerWithAglo) Op(state *testState) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, i)
	kp, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat(b, 4)))
	if err != nil {
		state.t.Fatal(err)
	}
	kp.Id.Algo = op.algo

	publisher := newPublisherWithKP(state.t, state.store.root, state.store.userLogs, kp)
	state.peers[op.name] = publisher
	ref := publisher.key.Id.Ref()
	state.refToName[ref] = op.name
	state.t.Logf("created(%d) %s as %s (algo:%s)", i, op.name, ref, op.algo)
	i++
	return nil
}

func getAliceBob(a, b string, state *testState) (*publisher, *publisher, error) {
	alice, ok := state.peers[a]
	if !ok {
		return nil, nil, fmt.Errorf("no such from peer")
	}
	bob, ok := state.peers[b]
	if !ok {
		return nil, nil, fmt.Errorf("no such wanted peer")
	}
	return alice, bob, nil
}

type PeopleOpFollow struct {
	p, wants string
}

func (op PeopleOpFollow) Op(state *testState) error {
	alice, w, err := getAliceBob(op.p, op.wants, state)
	if err != nil {
		return err
	}
	alice.follow(w.key.Id)
	return nil
}

type PeopleOpUnfollow struct {
	p, wants string
}

func (op PeopleOpUnfollow) Op(state *testState) error {
	alice, w, err := getAliceBob(op.p, op.wants, state)
	if err != nil {
		return err
	}
	alice.unfollow(w.key.Id)
	return nil
}

type PeopleOpBlock struct {
	p, wants string
}

func (op PeopleOpBlock) Op(state *testState) error {
	alice, w, err := getAliceBob(op.p, op.wants, state)
	if err != nil {
		return err
	}
	alice.block(w.key.Id)
	return nil
}

type PeopleOpUnblock struct {
	p, wants string
}

func (op PeopleOpUnblock) Op(state *testState) error {
	alice, w, err := getAliceBob(op.p, op.wants, state)
	if err != nil {
		return err
	}
	alice.unblock(w.key.Id)
	return nil
}

type PeopleAssert func(Builder) error

func PeopleAssertPathDist(from, to string, hops int) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(from, to, state)
		return func(bld Builder) error {
			if err != nil {
				return errors.Wrap(err, "dist: no such peers")
			}
			g, err := bld.Build()
			if err != nil {
				return err
			}
			dijk, err := g.MakeDijkstra(a.key.Id)
			if err != nil {
				return errors.Wrap(err, "dist: make dijkstra failed")
			}

			path, dist := dijk.Dist(b.key.Id)
			if len(path)-2 != hops {
				return errors.Errorf("wrong hop count: %v %f", path, dist)
			}
			return nil
		}
	}
}

func PeopleAssertFollows(from, to string, want bool) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(from, to, state)
		return func(bld Builder) error {
			if err != nil {
				return errors.Wrap(err, "follows: no such peers")
			}
			g, err := bld.Build()
			if err != nil {
				return err
			}
			if g.Follows(a.key.Id, b.key.Id) != want {
				return errors.Errorf("follows assert failed - wanted %v", want)
			}
			return nil
		}
	}
}

func PeopleAssertBlocks(from, to string, want bool) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(from, to, state)
		return func(bld Builder) error {
			if err != nil {
				return errors.Wrap(err, "blocks: no such peers")
			}
			g, err := bld.Build()
			if err != nil {
				return err
			}
			if g.Blocks(a.key.Id, b.key.Id) != want {
				return errors.Errorf("Blocks() assert failed - wanted %v", want)
			}
			set := g.BlockedList(a.key.Id)
			if isBlocked, has := set[b.key.Id.StoredAddr()]; isBlocked != want {
				return errors.Errorf("BlockedList() assert failed - wanted %v (has: %v)", want, has)
			}
			return nil
		}
	}
}

func PeopleAssertAuthorize(host, remote string, hops int, want bool) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(host, remote, state)
		return func(bld Builder) error {
			if err != nil {
				return errors.Wrap(err, "auth: no such peers")
			}

			auth := bld.Authorizer(a.key.Id, hops)

			err := auth.Authorize(b.key.Id)
			if want {
				if err != nil {
					return errors.Errorf("auth assert: %s didn't allow %s", host, remote)
				}
				return nil
			}
			if err == nil {
				return errors.Errorf("auth assert: host(%s) accepted remote(%s) (dist:%d)", host, remote, hops)
			}
			// TODO compare err?
			return nil
		}
	}
}

type PeopleAssertMaker func(*testState) PeopleAssert

type PeopleTestCase struct {
	name    string
	ops     []PeopleOp
	asserts []PeopleAssertMaker
}

func (tc PeopleTestCase) run(mk func(t *testing.T) testStore) func(t *testing.T) {
	return func(t *testing.T) {
		r := require.New(t)
		a := assert.New(t)

		var state testState
		state.peers = make(map[string]*publisher)
		state.refToName = make(map[string]string)
		state.store = mk(t)
		state.t = t

		for i, op := range tc.ops {
			err := op.Op(&state)
			r.NoError(err, "error performing operation(%d) of %v type %T: %s", i, op, op)
		}

		// punch in nicks
		g, err := state.store.gbuilder.Build()
		r.NoError(err, "failed to build graph for debugging")
		g.Lock()
		for nick, pub := range state.peers {
			newKey := pub.key.Id.StoredAddr()
			// var newKey [32]byte
			// copy(newKey[:], pub.key.Id.StoredAddr())
			node, ok := g.lookup[newKey]
			if !a.True(ok, "did not find peer!? %s (len:%d)", nick, len(g.lookup)) {
				continue
			}
			cn := node.(*contactNode)
			cn.name = nick
		}
		g.Unlock()

		for i, assert := range tc.asserts {
			err := assert(&state)(state.store.gbuilder)
			if !a.NoError(err, "assertion #%d failed", i) {

				err = g.RenderSVGToFile(fmt.Sprintf("%s-%d.svg", t.Name(), i))
				if err != nil {
					t.Log("warning: failed to dump graph to SVG", err)
				}
			}
		}

		state.store.close()
	}
}

func TestPeople(t *testing.T) {
	tcs := []PeopleTestCase{
		{
			name: "simple",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpNewPeer{"claire"},
				PeopleOpFollow{"alice", "bob"},
				PeopleOpFollow{"alice", "claire"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", true),
				PeopleAssertFollows("alice", "claire", true),
				PeopleAssertFollows("bob", "alice", false),
				PeopleAssertFollows("bob", "claire", false),

				PeopleAssertAuthorize("alice", "bob", 0, true),
				PeopleAssertAuthorize("bob", "alice", 0, false),
			},
		},

		{
			name: "unfollow",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpFollow{"alice", "bob"},
				PeopleOpUnfollow{"alice", "bob"},

				// make sure other connections still work
				PeopleOpNewPeer{"claire"},
				PeopleOpFollow{"alice", "claire"},
				PeopleOpFollow{"claire", "bob"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", false),
				PeopleAssertFollows("bob", "alice", false),

				PeopleAssertAuthorize("alice", "bob", 0, false),

				PeopleAssertAuthorize("alice", "claire", 0, true),
				PeopleAssertFollows("claire", "bob", true),
			},
		},

		{
			name: "same",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},

				PeopleOpFollow{"alice", "alice"}, // might happen but shouldn't be a problem
				PeopleOpFollow{"alice", "bob"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", true),
				PeopleAssertFollows("bob", "alice", false),

				PeopleAssertAuthorize("alice", "bob", 0, true),
			},
		},

		{
			name: "friends",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpNewPeer{"claire"},
				// friends
				PeopleOpFollow{"alice", "bob"},
				PeopleOpFollow{"bob", "alice"},

				PeopleOpFollow{"bob", "claire"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", true),
				PeopleAssertFollows("bob", "alice", true),
				PeopleAssertFollows("alice", "claire", false),
				PeopleAssertPathDist("alice", "claire", 1),

				PeopleAssertAuthorize("alice", "bob", 0, true),
				PeopleAssertAuthorize("bob", "alice", 0, true),

				PeopleAssertAuthorize("alice", "claire", 0, false),
				PeopleAssertAuthorize("alice", "claire", 1, true),
			},
		},

		{
			name: "friends2",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpNewPeer{"claire"},
				PeopleOpNewPeer{"debora"},
				// friends
				PeopleOpFollow{"alice", "bob"},
				PeopleOpFollow{"bob", "alice"},

				// friends
				PeopleOpFollow{"bob", "claire"},
				PeopleOpFollow{"claire", "bob"},

				PeopleOpFollow{"claire", "debora"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", true),
				PeopleAssertFollows("bob", "alice", true),
				PeopleAssertFollows("bob", "claire", true),
				PeopleAssertPathDist("alice", "debora", 2),

				PeopleAssertAuthorize("alice", "debora", 0, false),
				PeopleAssertAuthorize("alice", "debora", 1, false),
				PeopleAssertAuthorize("alice", "debora", 2, true),
			},
		},

		{
			name: "blocks",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpBlock{"alice", "bob"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", false),
				PeopleAssertBlocks("alice", "bob", true),
			},
		},

		{
			name: "unblock",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpBlock{"alice", "bob"},
				PeopleOpUnblock{"alice", "bob"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", false),
				PeopleAssertBlocks("alice", "bob", false),
			},
		},

		{
			name: "feedFormats",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"claire"},

				PeopleOpNewPeerWithAglo{"piet", ssb.RefAlgoFeedGabby},
				PeopleOpNewPeerWithAglo{"pew", ssb.RefAlgoFeedGabby},

				PeopleOpFollow{"alice", "piet"},
				PeopleOpFollow{"piet", "claire"},
				PeopleOpFollow{"piet", "pew"},
				PeopleOpFollow{"pew", "piet"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "piet", true),
				PeopleAssertFollows("piet", "alice", false),
				PeopleAssertFollows("piet", "claire", true),

				PeopleAssertFollows("piet", "pew", true),
				PeopleAssertFollows("pew", "piet", true),

				PeopleAssertAuthorize("alice", "piet", 0, true),
				PeopleAssertAuthorize("piet", "alice", 0, false),
				PeopleAssertAuthorize("piet", "claire", 0, true),
				PeopleAssertAuthorize("piet", "pew", 0, true),
				PeopleAssertAuthorize("pew", "piet", 0, true),

				PeopleAssertHops("alice", 0, "alice", "piet"),
				PeopleAssertHops("piet", 0, "piet", "claire", "pew"),
			},
		},

		// { TODO: unfinished
		// 	name: "inviteAccept",
		// 	ops: []PeopleOp{
		// 		PeopleOpNewPeer{"alice"},
		// 		PeopleOpNewPeer{"bob"},
		// 		PeopleOpNewPeer{"claire"},

		// 		// friends
		// 		PeopleOpFollow{"alice", "bob"},
		// 		PeopleOpFollow{"bob", "alice"},

		// 		// bob invites claire
		// 		PeopleOpInvites{"bob", "claire"},
		// 	},
		// 	asserts: []PeopleAssertMaker{
		// 		// follow setup
		// 		PeopleAssertFollows("alice", "bob", true),
		// 		PeopleAssertFollows("bob", "alice", true),

		// 		// invite confirm interpretation
		// 		PeopleAssertFollows("bob", "claire", true),
		// 		PeopleAssertFollows("claire", "bob", true),
		// 		PeopleAssertPathDist("alice", "claire", 2),

		// 		PeopleAssertAuthorize("alice", "claire", 0, false),
		// 		PeopleAssertAuthorize("alice", "claire", 1, true),
		// 		PeopleAssertAuthorize("alice", "claire", 2, true),
		// 	},
		// },
	}

	tcs = append(tcs, blockScenarios...)
	tcs = append(tcs, hopsScenarios...)

	for _, tc := range tcs {
		t.Run(tc.name+"/badger", tc.run(makeBadger))
		// t.Run(tc.name+"/tlog", tc.run(makeTypedLog))
	}
}

type PeopleOpInvites struct {
	author, receiver string
}

func (op PeopleOpInvites) Op(state *testState) error {
	alice, w, err := getAliceBob(op.author, op.receiver, state)
	if err != nil {
		return err
	}
	newSeq, err := alice.publish.Publish(map[string]interface{}{
		"type":      "contact",
		"contact":   w,
		"following": false,
	})

	alice.r.NoError(err)
	alice.r.NotNil(newSeq)
	return nil
}
