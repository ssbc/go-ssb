package graph

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type PeopleOp interface {
	Op(*testState) error
}

type testState struct {
	t     *testing.T
	peers map[string]*publisher
	store testStore
}

type PeopleOpNewPeer struct {
	name string
}

func (op PeopleOpNewPeer) Op(state *testState) error {
	publisher := newPublisher(state.t, state.store.root, state.store.userLogs)
	state.peers[op.name] = publisher
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

type PeopleAssert func(*Graph) error

func PeopleAssertPathDist(from, to string, hops int) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(from, to, state)
		return func(g *Graph) error {
			if err != nil {
				return errors.Wrap(err, "dist: no such peers")
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
		return func(g *Graph) error {
			if err != nil {
				return errors.Wrap(err, "dist: no such peers")
			}

			if g.Follows(a.key.Id, b.key.Id) != want {
				return errors.Errorf("follows assert failed - wanted %v", want)
			}
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
		state.store = mk(t)
		state.t = t

		for i, op := range tc.ops {
			err := op.Op(&state)
			r.NoError(err, "error performing operation(%d) of %v type %T: %s", i, op, op)
		}
		g, err := state.store.gbuilder.Build()
		r.NoError(err, "failed to build graph for asserts")
		for i, assert := range tc.asserts {
			err := assert(&state)(g)
			a.NoError(err, "assertion #%d failed", i)

			if t.Failed() {
				for nick, pub := range state.peers {
					var newKey [32]byte
					copy(newKey[:], pub.key.Id.ID)
					node, ok := g.lookup[newKey]
					r.True(ok, "did not find peer!?")
					cn := node.(*contactNode)
					cn.name = nick
				}
				err = g.RenderSVGToFile(fmt.Sprintf("%s-%d.svg", t.Name(), i))
				r.NoError(err)
			}
		}
	}
}

func TestPeople(t *testing.T) {
	tcs := []PeopleTestCase{
		{
			name: "simple",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpFollow{"alice", "bob"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", true),
				PeopleAssertFollows("bob", "alice", false),
			},
		},

		{
			name: "unfollow",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpFollow{"alice", "bob"},
				PeopleOpUnfollow{"alice", "bob"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertFollows("alice", "bob", false),
				PeopleAssertFollows("bob", "alice", false),
				// PeopleAssertPathDist("alice", "bob", 999),
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
			},
		},

		{
			name: "blocks",
			ops: []PeopleOp{
				PeopleOpNewPeer{"alice"},
				PeopleOpNewPeer{"bob"},
				PeopleOpFollow{"alice", "bob"},
				PeopleOpUnfollow{"alice", "bob"},
			},
			asserts: []PeopleAssertMaker{
				PeopleAssertPathDist("alice", "bob", 9999),
			},
		},
	}

	for _, tc := range tcs {
		t.Run("badger/"+tc.name, tc.run(makeBadger))
		t.Run("tlog/"+tc.name, tc.run(makeTypedLog))
	}
}
