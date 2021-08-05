// SPDX-License-Identifier: MIT

package graph

import (
	"fmt"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metakeys"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	refs "go.mindeco.de/ssb-refs"
)

var metafeedsScenarios = []PeopleTestCase{
	{
		name: "metafeeds, simple",
		ops: []PeopleOp{
			PeopleOpNewPeerWithAlgo{"alice", refs.RefAlgoFeedBendyButt},

			PeopleOpNewSubFeed{
				of:    "alice",
				name:  "alice-legacy",
				nonce: "test1",
				algo:  refs.RefAlgoFeedSSB1,
			},
			PeopleOpNewSubFeed{
				of:    "alice",
				name:  "alice-gabby",
				nonce: "test2",
				algo:  refs.RefAlgoFeedGabby},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertIsSubfeed("alice", "alice-legacy", true),
			PeopleAssertIsSubfeed("alice", "alice-gabby", true),
			PeopleAssertHops("alice", 0, "alice-legacy", "alice-gabby"),
		},
	},

	{
		name: "metafeeds of a friend",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeerWithAlgo{"bob", refs.RefAlgoFeedBendyButt},

			// alice and bob are friends
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"bob", "alice"},

			// bobs subfeeds
			PeopleOpNewSubFeed{of: "bob",
				name:  "bob-legacy",
				nonce: "test1",
				algo:  refs.RefAlgoFeedSSB1},

			PeopleOpNewSubFeed{of: "bob",
				name:  "bob-gabby",
				nonce: "test2",
				algo:  refs.RefAlgoFeedGabby},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("alice", 0, "bob", "bob-legacy", "bob-gabby"),
		},
	},

	{
		name: "metafeed follows someone",
		ops: []PeopleOp{
			PeopleOpNewPeerWithAlgo{"alice", refs.RefAlgoFeedBendyButt},
			PeopleOpNewPeer{"some"},

			PeopleOpNewSubFeed{
				of:    "alice",
				name:  "alice-legacy",
				nonce: "test1",
				algo:  refs.RefAlgoFeedSSB1,
			},

			PeopleOpFollow{"alice-legacy", "some"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("alice", 0, "alice-legacy", "some"),
		},
	},
}

type PeopleOpNewSubFeed struct {
	of, nonce, name string
	algo            refs.RefAlgo
}

func (op PeopleOpNewSubFeed) Op(state *testState) error {
	owningFeed, ok := state.peers[op.of]
	if !ok {
		return fmt.Errorf("no such of peer: %s", op.of)
	}

	kp, ok := owningFeed.key.(metakeys.KeyPair)
	if !ok {
		return fmt.Errorf("wrong keypair type: %T", owningFeed.key)
	}

	subKeyPair, err := metakeys.DeriveFromSeed(kp.Seed, op.nonce, op.algo)
	if err != nil {
		return fmt.Errorf("failed to create keypair: %w", err)
	}

	addContent := metamngmt.NewAddMessage(owningFeed.key.ID(), subKeyPair.Feed, op.nonce, []byte(op.nonce))

	addMsg, err := metafeed.SubSignContent(subKeyPair.PrivateKey, addContent, nil)
	if err != nil {
		return err
	}

	_, err = owningFeed.publish.Append(addMsg)
	if err != nil {
		return err
	}

	publisher := newPublisherWithKP(state.t, state.store.root, state.store.userLogs, subKeyPair)
	state.peers[op.name] = publisher
	ref := publisher.key.ID().Ref()
	state.refToName[ref] = op.name
	state.t.Logf("created(%d) %s as %s (algo:%s) as subfeed of %s ", i, op.name, ref, op.algo, op.of)
	i++

	return nil
}

func PeopleAssertIsSubfeed(of, subfeed string, want bool) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(of, subfeed, state)
		return func(bld Builder) error {
			if err != nil {
				return fmt.Errorf("subfeed: no such peers: %w", err)
			}
			g, err := bld.Build()
			if err != nil {
				return err
			}
			if g.Subfeed(a.key.ID(), b.key.ID()) != want {
				return fmt.Errorf("subfeed assert failed - wanted %v", want)
			}
			return nil
		}
	}
}
