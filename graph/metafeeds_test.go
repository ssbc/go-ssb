// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package graph

import (
	"fmt"

	"strings"
	"go.cryptoscope.co/ssb"
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

		// name: "main announces fake metafeed, should not work",
		// name: "followed peer has newly announced metafeed",
	{
		name: "follow peer with metafeed announcement",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice-main"},
			PeopleOpNewPeer{"bob-main"},
			PeopleOpFollow{"alice-main", "bob-main"},
			PeopleOpNewPeerWithAlgo{"bob-mf", refs.RefAlgoFeedBendyButt},
			PeopleOpMetafeedAddExisting{"bob-main", "bob-mf"},
			PeopleOpAnnounceMetafeed{"bob-main", "bob-mf"},
			// question: do we need to add a new op for adding an existing subfeed?
			// i.e. PeopleOpExistingSubFeed, instead of PeopleOpNewSubFeed

			// question: how do we actually test that the announcement works correctly?
			// i assume we need to create a normal feed (normie) following bob-main, and then check that 
			// the hops from normie->{bob-mf,bob-main,bob-indexes: about} == 0 (using hops 0 according to go-ssb hops conventions)
			//
			// following that logic: we should also have a test that maliciously creates an announcement for a metafeed that
			// they do not own, and make sure that situation is not replicated
			PeopleOpNewSubFeed{
				of:    "bob-mf",
				name:  "bob-indexes: about",
				nonce: "test1",
				algo:  refs.RefAlgoFeedSSB1,
			},
		},
		/* note: in go-ssb hops are one less than the equivalent in nodejs */
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("bob-mf", 0, "bob-main", "bob-indexes: about"),
			PeopleAssertHops("alice-main", 0, "bob-main", "bob-mf", "bob-indexes: about"),
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

	/* metafeed's corresponding ackowledgement of the announcement
	in bendybutt format
  "type" => "metafeed/add/existing",
  "feedpurpose" => "main",
  "subfeed" => (BFE-encoded feed ID for the 'main' feed),
  "metafeed" => (BFE-encoded Bendy Butt feed ID for the meta feed),
  "tangles" => {
    "metafeed" => {
      "root" => (BFE nil),
      "previous" => (BFE nil)
    }
  }
	*/
	// TODO: create a bendybutt message on root metafeed tying the mf and main feeds together
	// sign the bendybutt message with mf.Secret + main.Secret

	// mfAddExisting <=> "metafeed/add/existing"
	mfAddExisting := metamngmt.NewAddExistingMessage(kpMetafeed.ID(), kpMain.ID(), "main")
	mfAddExisting.Tangles["metafeed"] = refs.TanglePoint{Root: nil, Previous: nil}

	signedAddExistingContent, err := metafeed.SubSignContent(kpMain.Secret(), mfAddExisting)
	if err != nil {
		return err
	}
	mf.publish.Append(signedAddExistingContent)

	return nil
}

type metafeedAnnounceMsg struct {
	MsgType string `json:"type"`
	Metafeed string `json:"metafeed"`
	Tangles refs.TanglePoint
}

func safeSSBURI (input string) string {
	input = strings.ReplaceAll(input, "+", "-")
	input = strings.ReplaceAll(input, "/", "_")
	// TODO: remove kludge
	return strings.TrimSuffix(input, ".bbfeed-v1")
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

	kpMain, ok := mainFeed.key.(ssb.KeyPair)
	if !ok {
		return fmt.Errorf("wrong keypair type for main: %T", mainFeed.key)
	}
	kpMetafeed, ok := mf.key.(metakeys.KeyPair)
	if !ok {
		return fmt.Errorf("wrong keypair type for mf: %T", mf.key)
	}

	/* message structure for the announcement
			content: {
			  type: 'metafeed/announce',
			  metafeed: 'ssb:feed/bendybutt-v1/-oaWWDs8g73EZFUMfW37R_ULtFEjwKN_DczvdYihjbU=',
			  tangles: {
			  	metafeed: {
			  		root: null,
			  		previous: null
			  	}
			  }
		}
	*/

	// construct announcement message according to the JSON template above 
	// TODO? replace with https://godocs.io/github.com/ssb-ngi-pointer/go-metafeed/metamngmt#Announce 
	var announcement metafeedAnnounceMsg
	announcement.MsgType = "metafeed/announce"
	// TODO: replace safeSSBURI with cryptix's ref->Sigil/URI PR
	// TODO: just use <metafeedRef>.String() when ssb uri rewrite lands
	announcement.Metafeed = fmt.Sprintf("ssb:feed/bendybutt-v1/%s", safeSSBURI(kpMetafeed.ID().Ref()))
	announcement.Tangles.Root = nil
	announcement.Tangles.Previous = nil

	_, err = mainFeed.publish.Append(announcement)
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
