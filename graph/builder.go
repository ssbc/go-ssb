package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os/exec"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"

	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/message"
)

type Builder interface {
	librarian.SinkIndex

	Build() (*Graph, error)
	Follows(*sbot.FeedRef) ([]*sbot.FeedRef, error)
}

type builder struct {
	librarian.SinkIndex
	kv  *badger.DB
	idx librarian.Index
	log log.Logger
}

func NewBuilder(log log.Logger, db *badger.DB) Builder {
	contactsIdx := libbadger.NewIndex(db, 0)

	return &builder{
		kv:  db,
		idx: contactsIdx,
		log: log,

		SinkIndex: librarian.NewSinkIndex(func(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
			msg := val.(message.StoredMessage)
			var dmsg message.DeserializedMessage
			err := json.Unmarshal(msg.Raw, &dmsg)
			if err != nil {
				return errors.Wrap(err, "db/idx contacts: first json unmarshal failed")
			}

			var c sbot.Contact
			err = json.Unmarshal(dmsg.Content, &c)
			if err != nil {
				if sbot.IsMessageUnusable(err) {
					return nil
				}
				log.Log("msg", "skipped contact message", "reason", err)
				return nil
			}

			addr := append(dmsg.Author.ID, ':')
			addr = append(addr, c.Contact.ID...)
			switch {
			case c.Following:
				err = idx.Set(ctx, librarian.Addr(addr), 1)
			case c.Blocking:
				err = idx.Set(ctx, librarian.Addr(addr), 0)
			default:
				err = idx.Delete(ctx, librarian.Addr(addr))
			}
			if err != nil {
				return errors.Wrapf(err, "db/idx contacts: failed to update index. %+v", c)
			}

			return err
		}, contactsIdx),
	}
}

func (b *builder) Build() (*Graph, error) {
	dg := simple.NewWeightedDirectedGraph(0, math.Inf(1))
	known := make(key2node)

	err := b.kv.View(func(txn *badger.Txn) error {
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
				ID:   k[33:],
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
				w = math.Inf(1)
			}
			if nFrom.ID() == nTo.ID() {
				fmt.Printf("skipping self: %s\n", from.Ref())
				continue
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

func (g *Graph) Follows(from, to *sbot.FeedRef) bool {
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

func (b *builder) Follows(fr *sbot.FeedRef) ([]*sbot.FeedRef, error) {
	var friends []*sbot.FeedRef
	err := b.kv.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		prefix := append(fr.ID, ':')

		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			it := iter.Item()
			k := it.Key()
			c := sbot.FeedRef{
				Algo: "ed25519",
				ID:   k[33:],
			}
			v, err := it.Value()
			if err != nil {
				return errors.Wrap(err, "friends: counldnt get idx value")
			}
			if len(v) >= 1 && v[0] == '1' {
				friends = append(friends, &c)
			}
		}
		return nil
	})

	return friends, err
}

type contactNode struct {
	graph.Node
	name string
}

func (n contactNode) String() string {
	return n.name[0:5]
}

type key2node map[[32]byte]graph.Node

func Authorize(log log.Logger, b Builder, local *sbot.FeedRef, maxHops int, makeHandler func(net.Conn) (muxrpc.Handler, error)) func(net.Conn) (muxrpc.Handler, error) {
	return func(conn net.Conn) (muxrpc.Handler, error) {
		remote, err := sbot.GetFeedRefFromAddr(conn.RemoteAddr())
		if err != nil {
			return nil, errors.Wrap(err, "MakeHandler: expected an address containing an shs-bs addr")
		}

		// TODO: cache me in tandem with indexing
		timeGraph := time.Now()

		fg, err := b.Build()
		if err != nil {
			return nil, errors.Wrap(err, "MakeHandler: failed to make friendgraph")
		}

		if fg.Nodes() != 0 { // trust on first use
			timeDijkstra := time.Now()

			if fg.Follows(local, remote) {
				// quick skip direct follow
				return makeHandler(conn)
			}

			distLookup, err := fg.MakeDijkstra(local)
			if err != nil {
				return nil, errors.Wrap(err, "MakeHandler: failed to construct dijkstra")
			}
			timeLookup := time.Now()

			fpath, d := distLookup.Dist(remote)
			timeDone := time.Now()

			log.Log("event", "disjkstra",
				"nodes", fg.Nodes(),

				"total", timeDone.Sub(timeGraph),
				"lookup", timeDone.Sub(timeLookup),
				"mkGraph", timeDijkstra.Sub(timeGraph),
				"mkSearch", timeLookup.Sub(timeDijkstra),

				"dist", d,
				"hops", len(fpath),
				"path", fmt.Sprint(fpath),

				"remote", remote,
			)

			if d < 0 && len(fpath) < maxHops {
				return nil, errors.Errorf("sbot: peer not in reach. d:%d, max:%d", d, maxHops)
			}
		}

		return makeHandler(conn)
	}
}
