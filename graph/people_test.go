// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package graph

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
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
	var opk PeopleOpNewPeerWithAlgo
	opk.name = op.name
	opk.algo = refs.RefAlgoFeedSSB1
	return opk.Op(state)
}

type PeopleOpNewPeerWithAlgo struct {
	name string
	algo refs.RefAlgo
}

var i uint64

func (op PeopleOpNewPeerWithAlgo) Op(state *testState) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, i)
	kp, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat(b, 4)), op.algo)
	if err != nil {
		state.t.Fatal(err)
	}

	publisher := newPublisherWithKP(state.t, state.store.root, state.store.userLogs, kp)
	state.peers[op.name] = publisher
	ref := publisher.key.ID().String()
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
	alice.follow(w.key.ID())
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
	alice.unfollow(w.key.ID())
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
	alice.block(w.key.ID())
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
	alice.unblock(w.key.ID())
	return nil
}

type PeopleAssert func(Builder) error

func PeopleAssertPathDist(from, to string, hops int) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(from, to, state)
		return func(bld Builder) error {
			if err != nil {
				return fmt.Errorf("dist: no such peers: %w", err)
			}
			g, err := bld.Build()
			if err != nil {
				return err
			}
			dijk, err := g.MakeDijkstra(a.key.ID())
			if err != nil {
				return fmt.Errorf("dist: make dijkstra failed: %w", err)
			}

			path, dist := dijk.Dist(b.key.ID())
			if hops < 0 {
				if !math.IsInf(dist, +1) {
					return fmt.Errorf("expected no path but got %v %f", path, dist)
				}

			} else {
				if len(path)-2 != hops {
					return fmt.Errorf("wrong hop count: %v %f", path, dist)
				}
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
				return fmt.Errorf("follows: no such peers: %w", err)
			}
			g, err := bld.Build()
			if err != nil {
				return err
			}
			if g.Follows(a.key.ID(), b.key.ID()) != want {
				return fmt.Errorf("follows assert failed - wanted %v", want)
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
				return fmt.Errorf("blocks: no such peers: %w", err)
			}
			g, err := bld.Build()
			if err != nil {
				return err
			}
			if g.Blocks(a.key.ID(), b.key.ID()) != want {
				return fmt.Errorf("Blocks() assert failed - wanted %v", want)
			}
			set := g.BlockedList(a.key.ID())
			isBlocked := set.Has(b.key.ID())
			if isBlocked != want {
				return fmt.Errorf("BlockedList() assert failed - wanted %v (has: %v)", want, isBlocked)
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
				return fmt.Errorf("auth: no such peers: %w", err)
			}

			auth := bld.Authorizer(a.key.ID(), hops)

			err := auth.Authorize(b.key.ID())
			if want {
				if err != nil {
					return fmt.Errorf("auth assert: %s didn't allow %s", host, remote)
				}
				return nil
			}
			if err == nil {
				return fmt.Errorf("auth assert: host(%s) accepted remote(%s) (dist:%d)", host, remote, hops)
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
			newKey := storedrefs.Feed(pub.key.ID())
			node, ok := g.lookup[newKey]
			if !a.True(ok, "did not find peer %s in graph lookup table (len:%d)", nick, len(g.lookup)) {
				continue
			}
			node.name = nick
		}
		g.Unlock()

		for i, assert := range tc.asserts {
			err := assert(&state)(state.store.gbuilder)
			if !a.NoError(err, "assertion #%d failed", i+1) {

				err = g.RenderSVGToFile(fmt.Sprintf("%s-%d.svg", t.Name(), i))
				if err != nil {
					t.Log("warning: failed to dump graph to SVG", err)
				}
			}
		}
	}
}

func TestPeople(t *testing.T) {
	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
		// TODO: expose index flushing
	}

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
		/*
			{
				name: "feedFormats",
				ops: []PeopleOp{
					PeopleOpNewPeer{"alice"},
					PeopleOpNewPeer{"claire"},

					PeopleOpNewPeerWithAlgo{"piet", ssb.RefAlgoFeedGabby},
					PeopleOpNewPeerWithAlgo{"pew", ssb.RefAlgoFeedGabby},

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
		*/

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
	tcs = append(tcs, metafeedsScenarios...)
	tcs = append(tcs, deleteScenarios...)

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
