package graph

import (
	"math"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
)

type authorizer struct {
	b       Builder
	from    *ssb.FeedRef
	maxHops int
	log     log.Logger
}

func (a *authorizer) Authorize(to *ssb.FeedRef) error {
	fg, err := a.b.Build()
	if err != nil {
		return errors.Wrap(err, "graph/Authorize: failed to make friendgraph")
	}

	if fg.Nodes() == 0 { // trust on first use
		return nil
	}

	if fg.Follows(a.from, to) {
		return nil
	}

	// TODO we need to check that `from` is in the graph, instead of checking if it's empty
	// only important in the _resync existing feed_ case. should maybe not construct this authorizer then?
	var distLookup *Lookup
	distLookup, err = fg.MakeDijkstra(a.from)
	if err != nil {
		// for now adding this as a kludge so that stuff works when you don't get your own feed during initial re-sync
		// if it's a new key there should be follows quickly anyway and this shouldn't happen then.... yikes :'(
		if _, ok := err.(*NoSuchFrom); ok {
			return nil
		}
		return errors.Wrap(err, "graph/Authorize: failed to construct dijkstra")
	}

	// dist includes start and end of the path so Alice to Bob will be
	// p:=[Alice, some, friends, Bob]
	// len(p) == 4
	_, d := distLookup.Dist(to)
	if math.IsInf(d, -1) || int(d) > a.maxHops {
		return &ssb.ErrOutOfReach{Dist: int(d), Max: a.maxHops}
	}

	return nil

}
