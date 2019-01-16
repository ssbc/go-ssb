package graph

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/traverse"
)

type contactNode struct {
	graph.Node
	feed *ssb.FeedRef
}

func (n contactNode) String() string {
	return n.feed.Ref()[:8]
}

type key2node map[[32]byte]graph.Node

type Graph struct {
	dg     *simple.WeightedDirectedGraph
	lookup key2node
}

func (g *Graph) Nodes() int {
	return len(g.lookup)
}

func (g *Graph) RenderSVG() error {
	dotbytes, err := dot.Marshal(g.dg, "", "", "")
	if err != nil {
		return errors.Wrap(err, "dot marshal failed")
	}
	dotCmd := exec.Command("dot", "-Tsvg", "-o", "fullgraph.svg")
	dotCmd.Stdin = bytes.NewReader(dotbytes)
	out, err := dotCmd.CombinedOutput()
	if err != nil {
		fmt.Println("dot out:", out)
		return errors.Wrap(err, "dot run failed")
	}
	return nil
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
	return g.dg.HasEdgeFromTo(nFrom.ID(), nTo.ID())
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
	w.Walk(g.dg, nFrom, func(n graph.Node, d int) bool {
		if d < plen {
			cn, ok := n.(contactNode)
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

type NoSuchFrom struct {
	*ssb.FeedRef
}

func (nsf NoSuchFrom) Error() string {
	return fmt.Sprintf("ssb/graph: no such from: %s", nsf.Ref())
}

func (g *Graph) MakeDijkstra(from *ssb.FeedRef) (*Lookup, error) {
	var bfrom [32]byte
	copy(bfrom[:], from.ID)
	nFrom, has := g.lookup[bfrom]
	if !has {
		return nil, &NoSuchFrom{from}
	}
	return &Lookup{
		path.DijkstraFrom(nFrom, g.dg),
		g.lookup,
	}, nil
}
