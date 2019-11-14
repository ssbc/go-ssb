// SPDX-License-Identifier: MIT

package graph

import (
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
)

/*
Quoting from https://github.com/ssbc/ssb-friends README.md

the relation between any two peers can be in 3 states. following, not following, and blocking.

- following means you will definitely replicate them.
- not following means you might not replicate them, but you might replicate them if your friend follows them.
- blocking means that you will not replicate them. if they are blocked by someone you follow, and you are not following them, then you will not replicate them.
- if a friend of blocks someone, they will not be replicated, unless another friend follows them.
- if one friend blocks, and another follows, they will be replicated but their friends won't be (this is to stop sybil swarms)

this description is awful! we need to reduce this
*/

var blockScenarios = []PeopleTestCase{
	{
		name: "block by friends",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},
			PeopleOpNewPeer{"claire"},

			// friends
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"bob", "alice"},

			PeopleOpBlock{"bob", "claire"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertFollows("alice", "bob", true),
			PeopleAssertFollows("bob", "alice", true),
			PeopleAssertBlocks("bob", "claire", true),

			PeopleAssertAuthorize("alice", "bob", 0, true),
			PeopleAssertAuthorize("alice", "claire", 0, false),
			PeopleAssertAuthorize("alice", "claire", 1, false),
			PeopleAssertAuthorize("alice", "claire", 2, false),

			PeopleAssertOnBlocklist("alice"),
			PeopleAssertOnBlocklist("bob", "claire"),
			PeopleAssertOnBlocklist("claire"),
		},
	},

	{
		name: "frenemy",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},
			PeopleOpNewPeer{"claire"},
			PeopleOpNewPeer{"debora"},

			// friend that blockes is bob
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"bob", "alice"},

			PeopleOpBlock{"bob", "debora"},

			// friend that follows is claire
			PeopleOpFollow{"alice", "claire"},
			PeopleOpFollow{"claire", "alice"},

			PeopleOpFollow{"claire", "debora"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertAuthorize("alice", "debora", 0, false),
			PeopleAssertAuthorize("alice", "debora", 1, true),
		},
	},

	{
		name: "frenemys friends",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},
			PeopleOpNewPeer{"claire"},
			PeopleOpNewPeer{"debora"},
			PeopleOpNewPeer{"edith"},

			// friend that blockes is bob
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"bob", "alice"},

			PeopleOpBlock{"bob", "debora"},

			// friend that follows is claire
			PeopleOpFollow{"alice", "claire"},
			PeopleOpFollow{"claire", "alice"},

			PeopleOpFollow{"claire", "debora"},

			PeopleOpFollow{"edith", "debora"},
			PeopleOpFollow{"debora", "edith"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertAuthorize("alice", "debora", 0, false),
			PeopleAssertAuthorize("alice", "debora", 1, true),

			PeopleAssertAuthorize("alice", "edith", 0, false),
			PeopleAssertAuthorize("alice", "edith", 1, false),

			// currently hop-count wins over _friend on path blocked this peer_
			// TODO: discuss at scuttle-camp how to treat these cases
			PeopleAssertAuthorize("alice", "edith", 2, true),
		},
	},
}

func PeopleAssertOnBlocklist(from string, who ...string) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {
		pFrom, ok := state.peers[from]
		if !ok {
			state.t.Fatal("no such wanted peer:", from)
			return nil
		}

		return func(bld Builder) error {

			g, err := bld.Build()
			if err != nil {
				return err
			}

			set := g.BlockedList(pFrom.key.Id)
			if len(set) != len(who) {
				return errors.Errorf("BlockedList() wrong length: %d", len(set))
			}

			var wants = make(map[librarian.Addr]struct{}, len(who))
			for _, want := range who {
				pFrom, ok := state.peers[want]
				if !ok {
					state.t.Fatal("no such wanted peer:", want)
					return nil
				}
				addr := pFrom.key.Id.StoredAddr()
				// state.t.Logf("%s: %x", want, addr)
				wants[addr] = struct{}{}
			}
			for blocked := range set {
				// state.t.Logf("%x", blocked)
				if _, isWant := wants[blocked]; !isWant {

					return errors.Errorf("BlockedList(): expected blocked peer: %x", blocked)
				}
			}
			return nil
		}
	}
}
