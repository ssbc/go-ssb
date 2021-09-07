// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package graph

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var hopsScenarios = []PeopleTestCase{
	{
		name: "negative hops",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},

			// alice is interested in
			PeopleOpFollow{"alice", "bob"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("alice", -1),
		},
	},

	{
		name: "hops 0",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},
			PeopleOpNewPeer{"claire"},

			// alice is interested in
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"alice", "claire"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("alice", 0, "bob", "claire"),
		},
	},

	{
		name: "hops 1",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},
			PeopleOpNewPeer{"claire"},

			// alice is interested in
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"alice", "claire"},

			// bobs friends
			PeopleOpNewPeer{"bobf1"},
			PeopleOpNewPeer{"bobf2"},
			PeopleOpFollow{"bob", "bobf1"},
			PeopleOpFollow{"bob", "bobf2"},

			PeopleOpFollow{"bob", "alice"},

			// claire is not friends with alice
			PeopleOpNewPeer{"off"},
			PeopleOpNewPeer{"off2"},
			PeopleOpFollow{"claire", "off"},
			PeopleOpFollow{"claire", "off2"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("alice", 0, "bob", "claire"),
			PeopleAssertHops("alice", 1, "bob", "claire", "bobf1", "bobf2"),
		},
	},

	{
		name: "hops 2",
		ops: []PeopleOp{
			PeopleOpNewPeer{"alice"},
			PeopleOpNewPeer{"bob"},
			PeopleOpNewPeer{"claire"},

			// alice is interested in
			PeopleOpFollow{"alice", "bob"},
			PeopleOpFollow{"alice", "claire"},

			// bobs friends
			PeopleOpNewPeer{"bobf1"},
			PeopleOpNewPeer{"bobf2"},
			PeopleOpFollow{"bob", "bobf1"},
			PeopleOpFollow{"bob", "bobf2"},

			PeopleOpFollow{"bob", "alice"},

			// hops 2
			PeopleOpNewPeer{"bobfam1"},
			PeopleOpNewPeer{"bobfam2"},
			PeopleOpNewPeer{"bobfam3"},
			PeopleOpFollow{"bobf1", "bobfam1"},
			PeopleOpFollow{"bobf1", "bobfam2"},
			PeopleOpFollow{"bobf1", "bobfam3"},

			PeopleOpFollow{"bobf1", "bob"},
		},
		asserts: []PeopleAssertMaker{
			PeopleAssertHops("alice", 0, "bob", "claire"),
			PeopleAssertHops("alice", 1, "bob", "claire", "bobf1", "bobf2"),
			PeopleAssertHops("alice", 2, "bob", "claire", "bobf1", "bobf2", "bobfam1", "bobfam2", "bobfam3"),
		},
	},
}

func PeopleAssertHops(from string, hops int, tos ...string) PeopleAssertMaker {
	return func(state *testState) PeopleAssert {

		return func(bld Builder) error {

			alice, ok := state.peers[from]
			if !ok {
				return fmt.Errorf("no such from peer")
			}

			hopSet := bld.Hops(alice.key.ID(), hops)
			require.NotNil(state.t, hopSet, "no hopSet for alice")
			assert.Equal(state.t, len(tos), hopSet.Count(), "set count incorrect")
			hopList, err := hopSet.List()
			if err != nil {
				return err
			}

			hitMap := make(map[string]bool, len(hopList))
			for _, h := range hopList {
				hitMap[h.String()] = false
			}
			// if n, m := len(hopList), len(tos); n != m {
			// 	return fmt.Errorf("count mismatch between want(%d) and got(%d)", m, n)
			// }
			for _, nick := range tos {
				bob, ok := state.peers[nick]
				if !ok {
					return fmt.Errorf("wanted peer not in known-peers list: %s", nick)
				}
				bobRef := bob.key.ID().String()

				_, ok = hitMap[bobRef]
				if !ok {
					dumpMap(hitMap, state)
					assert.True(state.t, ok, "wanted peer not in hops list: %s", nick)
				} else {
					hitMap[bobRef] = true
				}
			}

			var unwanted []string
			for k, hit := range hitMap {
				if !hit {
					unwanted = append(unwanted, state.refToName[k])
				}
			}
			if len(unwanted) > 0 {
				return fmt.Errorf("unwanted peers still in hit list: %v", unwanted)
			}
			return nil
		}
	}
}

func dumpMap(m map[string]bool, s *testState) {
	for k, v := range m {
		s.t.Logf("%v:%v", s.refToName[k], v)
	}
}
