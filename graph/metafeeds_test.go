// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package graph

import (
	"fmt"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metakeys"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/legacy"
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
		name: "metafeeds inside metafeeds",
		ops: []PeopleOp{
			PeopleOpNewPeerWithAlgo{"alice", refs.RefAlgoFeedBendyButt},

			PeopleOpNewSubFeed{
				of:    "alice",
				name:  "alice-indexes",
				nonce: "test1",
				algo:  refs.RefAlgoFeedBendyButt,
			},

			PeopleOpNewSubFeed{
				of:    "alice-indexes",
				name:  "alice-foo",
				nonce: "idx1",
				algo:  refs.RefAlgoFeedGabby,
			},
			PeopleOpNewSubFeed{
				of:    "alice-indexes",
				name:  "alice-bar",
				nonce: "idx2",
				algo:  refs.RefAlgoFeedSSB1,
			},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertIsSubfeed("alice", "alice-indexes", true),

			PeopleAssertIsSubfeed("alice-indexes", "alice-foo", true),
			PeopleAssertIsSubfeed("alice-indexes", "alice-bar", true),

			PeopleAssertHasMetafeed("alice-foo", "alice-indexes", true),
			PeopleAssertHasMetafeed("alice-bar", "alice-indexes", true),

			PeopleAssertHasMetafeed("alice-indexes", "alice", true),

			PeopleAssertHops("alice", 0, "alice-indexes", "alice-foo", "alice-bar"),
		},
	},

	{
		name: "metafeeds of a friend",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeerWithAlgo{"bob", refs.RefAlgoFeedBendyButt},

			// bobs subfeeds
			PeopleOpNewSubFeed{of: "bob",
				name:  "bob-legacy",
				nonce: "test1",
				algo:  refs.RefAlgoFeedSSB1,
			},

			PeopleOpNewSubFeed{of: "bob",
				name:  "bob-gabby",
				nonce: "test2",
				algo:  refs.RefAlgoFeedGabby,
			},

			// alice and bob are friends
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"bob-legacy", "alice"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("alice", 0, "bob", "bob-legacy", "bob-gabby"),
		},
	},

	{
		name: "metafeeds follow someone",
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

	{
		name: "follow peer with metafeed announcement",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice-main"},
			PeopleOpNewPeer{"bob-main"},
			PeopleOpFollow{"alice-main", "bob-main"},
			PeopleOpNewPeerWithAlgo{"bob-mf", refs.RefAlgoFeedBendyButt},
			PeopleOpAnnounceMetafeed{"bob-main", "bob-mf"},
			PeopleOpMetafeedAddExisting{"bob-main", "bob-mf"},

			// question: do we need to add a new op for adding an existing subfeed?
			// i.e. PeopleOpExistingSubFeed, instead of PeopleOpNewSubFeed

			// open question: we should also have a test that maliciously creates an announcement for a metafeed that they do
			// not own, and make sure that situation is not replicated
			PeopleOpNewSubFeed{
				of:    "bob-mf",
				name:  "bob-indexes (about)",
				nonce: "test1",
				algo:  refs.RefAlgoFeedSSB1,
			},
		},
		/* note: in go-ssb hops are one less than the equivalent in nodejs */
		asserts: []PeopleAssertMaker{
			PeopleAssertIsSubfeed("bob-mf", "bob-main", true),
			PeopleAssertIsSubfeed("bob-mf", "bob-indexes (about)", true),
			PeopleAssertHops("bob-mf", 0, "bob-main", "bob-indexes (about)"),
			PeopleAssertHops("alice-main", 0, "bob-main", "bob-mf", "bob-indexes (about)"),
		},
	},
}

type PeopleOpMetafeedAddExisting struct {
	main, mf string
}

func (op PeopleOpMetafeedAddExisting) Op(state *testState) error {
	var err error
	mainFeed, ok := state.peers[op.main]
	if !ok {
		return fmt.Errorf("no such main peer: %s", op.main)
	}
	mf, ok := state.peers[op.mf]
	if !ok {
		return fmt.Errorf("no such mf peer: %s", op.mf)
	}

	kpMain, ok := mainFeed.key.(ssb.KeyPair)
	if !ok {
		return fmt.Errorf("wrong keypair type for main: %T", mainFeed.key)
	}
	kpMetafeed, ok := mf.key.(metakeys.KeyPair)
	if !ok {
		return fmt.Errorf("wrong keypair type for mf: %T", mf.key)
	}

	// create a bendybutt message on the root metafeed, tying the mf and main feeds together, by publishing a message of type "metafeed/add/existing"
	mfAddExisting := metamngmt.NewAddExistingMessage(kpMetafeed.ID(), kpMain.ID(), "main")
	mfAddExisting.Tangles["metafeed"] = refs.TanglePoint{Root: nil, Previous: nil}

	// sign the bendybutt message with mf.Secret + main.Secret
	signedAddExistingContent, err := metafeed.SubSignContent(kpMain.Secret(), mfAddExisting)
	if err != nil {
		return err
	}

	newMsgRef, err := mf.publish.Append(signedAddExistingContent)
	if err != nil {
		return err
	}
	state.t.Log("published add/existing:", newMsgRef)

	return nil
}

type PeopleOpAnnounceMetafeed struct {
	main, mf string
}

func (op PeopleOpAnnounceMetafeed) Op(state *testState) error {
	var err error
	mainFeed, ok := state.peers[op.main]
	if !ok {
		return fmt.Errorf("no such main peer: %s", op.main)
	}
	mf, ok := state.peers[op.mf]
	if !ok {
		return fmt.Errorf("no such mf peer: %s", op.mf)
	}

	kpMetafeed, ok := mf.key.(metakeys.KeyPair)
	if !ok {
		return fmt.Errorf("wrong keypair type for mf: %T", mf.key)
	}

	// construct the announcement message according to spec
	announcement := legacy.NewMetafeedAnnounce(kpMetafeed.ID(), mainFeed.key.ID())
	signedAnnouncement, err := announcement.Sign(kpMetafeed.PrivateKey, nil)
	if err != nil {
		return err
	}

	_, err = mainFeed.publish.Append(signedAnnouncement)
	if err != nil {
		return err
	}
	return nil
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

	addContent := metamngmt.NewAddDerivedMessage(owningFeed.key.ID(), subKeyPair.Feed, op.nonce, []byte(op.nonce))

	addMsg, err := metafeed.SubSignContent(subKeyPair.PrivateKey, addContent)
	if err != nil {
		return err
	}

	_, err = owningFeed.publish.Append(addMsg)
	if err != nil {
		return err
	}

	publisher := newPublisherWithKP(state.t, state.store.root, state.store.userLogs, subKeyPair)
	state.peers[op.name] = publisher
	ref := publisher.key.ID().String()
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

func PeopleAssertHasMetafeed(sub, meta string, want bool) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		a, b, err := getAliceBob(sub, meta, state)
		return func(bld Builder) error {
			if err != nil {
				return fmt.Errorf("metafeed: no such peers: %w", err)
			}
			bb := bld.(*BadgerBuilder)

			found, err := bb.Metafeed(a.key.ID())
			if err != nil && want {
				return fmt.Errorf("metafeed: expected to find the metafeed (%w)", err)
			}

			wanted := found.Equal(b.key.ID())
			if wanted && !want {
				return fmt.Errorf("metafeed: did not expect to find the metafeed (%s)", found.String())
			}

			if !wanted {
				return fmt.Errorf("metafeed: found something but not the expected feed: %s", found.String())
			}

			return nil
		}
	}
}
