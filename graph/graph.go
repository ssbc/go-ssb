package graph

import (
	"go.cryptoscope.co/ssb"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/traverse"
)

type key2node map[[32]byte]graph.Node

type Graph struct {
	simple.WeightedDirectedGraph
	lookup key2node
}

func (g *Graph) Follows(from, to *ssb.FeedRef) bool {
	var bfrom [32]byte
	copy(bfrom[:], from.ID)
	nFrom, has := g.lookup[bfrom]
	if !has {
		return false
	}
	var bto [32]byte
	copy(bto[:], to.ID)
	nTo, has := g.lookup[bto]
	if !has {
		return false
	}
	if !g.HasEdgeFromTo(nFrom.ID(), nTo.ID()) {
		return false
	}
	edg := g.Edge(nFrom.ID(), nTo.ID())
	w := edg.(graph.WeightedEdge)
	return w.Weight() == 1
}

func (g *Graph) Hops(from *ssb.FeedRef, plen int) []*ssb.FeedRef {
	var bfrom [32]byte
	copy(bfrom[:], from.ID)
	nFrom, has := g.lookup[bfrom]
	if !has {
		return nil
	}
	walked := make(map[int64]*ssb.FeedRef, 5000)
	w := traverse.BreadthFirst{}
	w.Walk(g, nFrom, func(n graph.Node, d int) bool {
		if d < plen {
			cn, ok := n.(*contactNode)
			if ok {
				walked[cn.ID()] = cn.feed
			}
			return false
		}
		return true
	})
	var unique []*ssb.FeedRef
	for _, v := range walked {
		unique = append(unique, v)
	}
	return unique
}

func (g *Graph) MakeDijkstra(from *ssb.FeedRef) (*Lookup, error) {
	var bfrom [32]byte
	copy(bfrom[:], from.ID)
	nFrom, has := g.lookup[bfrom]
	if !has {
		return nil, &ErrNoSuchFrom{from}
	}
	return &Lookup{
		path.DijkstraFrom(nFrom, g),
		g.lookup,
	}, nil
}
