package repo

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/sbot"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/simple"
)

type contactNode struct {
	graph.Node
	name string
}

func (n contactNode) String() string {
	return n.name[0:5]
}

func (r *repo) Makegraph() error {
	dg := simple.NewDirectedGraph()
	known := make(map[[32]byte]graph.Node)

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
			if len(v) >= 1 && v[0] == '1' {
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

				dg.SetEdge(dg.NewEdge(nFrom, nTo))
			}
		}
		return nil
	})
	fmt.Println(len(known), "nodes")

	dotbytes, err := dot.Marshal(dg, "", "", "", true)
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
