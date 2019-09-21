package graph

import (
	"math"
	"sync"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/ssb"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
)

type key2node map[librarian.Addr]graph.Node

type Graph struct {
	sync.Mutex
	*simple.WeightedDirectedGraph
	lookup key2node
}

func (g *Graph) getEdge(from, to *ssb.FeedRef) (graph.WeightedEdge, bool) {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()
	nFrom, has := g.lookup[from.StoredAddr()]
	if !has {
		return nil, false
	}
	nTo, has := g.lookup[to.StoredAddr()]
	if !has {
		return nil, false
	}
	if !g.HasEdgeFromTo(nFrom.ID(), nTo.ID()) {
		return nil, false
	}
	edg := g.Edge(nFrom.ID(), nTo.ID())
	return edg.(graph.WeightedEdge), true
}

func (g *Graph) Follows(from, to *ssb.FeedRef) bool {
	w, has := g.getEdge(from, to)
	if !has {
		return false
	}
	return w.Weight() == 1
}

func (g *Graph) Blocks(from, to *ssb.FeedRef) bool {
	w, has := g.getEdge(from, to)
	if !has {
		return false
	}
	return w.Weight() == math.Inf(1)
}

func (g *Graph) BlockedList(from *ssb.FeedRef) map[librarian.Addr]bool {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()
	nFrom, has := g.lookup[from.StoredAddr()]
	if !has {
		return nil
	}
	blocked := make(map[librarian.Addr]bool)
	edgs := g.From(nFrom.ID())
	for edgs.Next() {
		edg := g.Edge(nFrom.ID(), edgs.Node().ID()).(contactEdge)

		if edg.Weight() == math.Inf(1) {
			ctNode := edg.To().(*contactNode)
			blocked[ctNode.feed.StoredAddr()] = true
		}
	}
	return blocked
}

func (g *Graph) MakeDijkstra(from *ssb.FeedRef) (*Lookup, error) {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()
	nFrom, has := g.lookup[from.StoredAddr()]
	if !has {
		return nil, &ErrNoSuchFrom{from}
	}
	return &Lookup{
		path.DijkstraFrom(nFrom, g),
		g.lookup,
	}, nil
}
