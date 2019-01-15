package graph

import (
	"math"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
)

type authorizer struct {
	b       *builder
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
	var distLookup *Lookup
	distLookup, err = fg.MakeDijkstra(a.from)
	if err != nil {
		return errors.Wrap(err, "graph/Authorize: failed to construct dijkstra")
	}

	_, d := distLookup.Dist(to)
	if math.IsInf(d, -1) || int(d) > a.maxHops {
		return &ssb.ErrOutOfReach{int(d), a.maxHops}
	}

	return nil

}
