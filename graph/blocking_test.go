package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
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
func (tc testStore) blockingScenario(t *testing.T) {
	r := require.New(t)

	g, err := tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(0, g.NodeCount())

	// this is _us_
	myself := tc.newPublisher(t)

	// our friend
	alice := tc.newPublisher(t)
	myself.follow(alice.key.Id)
	alice.follow(myself.key.Id)

	// friend of allice
	claire := tc.newPublisher(t)
	alice.follow(claire.key.Id)
	claire.follow(alice.key.Id)

	// followed by alice
	debby := tc.newPublisher(t)
	alice.follow(debby.key.Id)

	// bob can be complicated
	bob := tc.newPublisher(t)
	claire.block(bob.key.Id)
	myself.follow(bob.key.Id)

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(5, g.NodeCount())
	r.NoError(g.RenderSVGToFile("setup.svg"))

	// a := tc.gbuilder.Authorizer(myself.key.Id, 0)

}
