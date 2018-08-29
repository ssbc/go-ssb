package repo

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/sbot"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"
)

type contactNode struct {
	graph.Node
	name string
}

func (n contactNode) String() string {
	return n.name[0:5]
}

type key2node map[[32]byte]graph.Node

func (r *repo) Makegraph() (*Graph, error) {
	dg := simple.NewWeightedDirectedGraph(0, math.Inf(1))
	known := make(key2node)

	err := r.contactsKV.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		for iter.Rewind(); iter.Valid(); iter.Next() {
			it := iter.Item()
			k := it.Key()
			if len(k) < 65 {
				fmt.Printf("skipping: %q\n", string(k))
				continue
			}
			from := sbot.FeedRef{
				Algo: "ed25519",
				ID:   k[:32],
			}

			to := sbot.FeedRef{
				Algo: "ed25519",
				ID:   k[32:],
			}
			v, err := it.Value()
			if err != nil {
				return errors.Wrap(err, "friends: counldnt get idx value")
			}
			var bfrom [32]byte
			copy(bfrom[:], from.ID)
			nFrom, has := known[bfrom]
			if !has {
				nFrom = contactNode{dg.NewNode(), from.Ref()}
				dg.AddNode(nFrom)
				known[bfrom] = nFrom
			}
			var bto [32]byte
			copy(bto[:], to.ID)
			nTo, has := known[bto]
			if !has {
				nTo = contactNode{dg.NewNode(), to.Ref()}
				dg.AddNode(nTo)
				known[bto] = nTo
			}
			w := math.Inf(-1)
			if len(v) >= 1 && v[0] == '1' {
				w = 1
			} else {
				w = -1
			}
			edg := simple.WeightedEdge{F: nFrom, T: nTo, W: w}
			dg.SetWeightedEdge(edg)
		}
		return nil
	})

	// dg.HasEdgeBetween()

	return &Graph{dg, known}, err
}

type Graph struct {
	dg     *simple.WeightedDirectedGraph
	lookup key2node
}

func (g *Graph) Nodes() int {
	return len(g.lookup)
}

func (g *Graph) RenderSVG() error {
	dotbytes, err := dot.Marshal(g.dg, "", "", "", true)
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

func (g *Graph) IsFollowing(from, to *sbot.FeedRef) bool {
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

type Lookup struct {
	dijk   path.Shortest
	lookup key2node
}

func (l Lookup) Dist(to *sbot.FeedRef) ([]graph.Node, float64) {
	var bto [32]byte
	copy(bto[:], to.ID)
	nTo, has := l.lookup[bto]
	if !has {
		return nil, math.Inf(-1)
	}
	return l.dijk.To(nTo.ID())
}

func (g *Graph) MakeDijkstra(from *sbot.FeedRef) (*Lookup, error) {
	var bfrom [32]byte
	copy(bfrom[:], from.ID)
	nFrom, has := g.lookup[bfrom]
	if !has {
		return nil, errors.Errorf("make dijkstra: no such from: %s", from.Ref())
	}
	return &Lookup{
		path.DijkstraFrom(nFrom, g.dg),
		g.lookup,
	}, nil
}
