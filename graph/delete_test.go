// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package graph

import (
	"fmt"
)

var deleteScenarios = []PeopleTestCase{
	{
		name: "delete bob",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},
			PeopleOpNewPeer{"claire"},
			PeopleOpNewPeer{"dee"},

			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"bob", "alice"},

			PeopleOpFollow{"bob", "claire"},
			PeopleOpFollow{"claire", "dee"},

			PeopleOpDeleteAuthor{"bob"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertFollows("alice", "bob", true),
			PeopleAssertFollows("bob", "alice", false),
			PeopleAssertFollows("alice", "claire", false),
			PeopleAssertPathDist("alice", "claire", -1),

			PeopleAssertAuthorize("alice", "bob", 0, true),
			PeopleAssertAuthorize("bob", "alice", 0, false),

			PeopleAssertAuthorize("alice", "claire", 0, false),
			PeopleAssertAuthorize("alice", "claire", 1, false),
		},
	},
}

type PeopleOpDeleteAuthor struct {
	who string
}

func (op PeopleOpDeleteAuthor) Op(state *testState) error {
	who, ok := state.peers[op.who]
	if !ok {
		return fmt.Errorf("delete: no such who %s", op.who)
	}
	err := state.store.gbuilder.DeleteAuthor(who.key.ID())
	return err
}
