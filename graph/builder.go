package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

type Builder interface {
	librarian.SinkIndex

	Build() (*Graph, error)
	Follows(*ssb.FeedRef) ([]*ssb.FeedRef, error)
	Authorizer(from *ssb.FeedRef, maxHops int) ssb.Authorizer
}

type builder struct {
	librarian.SinkIndex
	kv  *badger.DB
	idx librarian.Index
	log log.Logger

	cacheLock   sync.Mutex
	cachedGraph *Graph
}

func NewBuilder(log log.Logger, db *badger.DB) Builder {
	contactsIdx := libbadger.NewIndex(db, 0)

	b := &builder{
		kv:  db,
		idx: contactsIdx,
		log: log,
	}

	b.SinkIndex = librarian.NewSinkIndex(func(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
		b.cacheLock.Lock()
		defer b.cacheLock.Unlock()

		if nulled, ok := val.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}

		msg := val.(message.StoredMessage)
		var dmsg message.DeserializedMessage
		err := json.Unmarshal(msg.Raw, &dmsg)
		if err != nil {
			return errors.Wrap(err, "db/idx contacts: first json unmarshal failed")
		}

		var c ssb.Contact
		err = json.Unmarshal(dmsg.Content, &c)
		if err != nil {
			if ssb.IsMessageUnusable(err) {
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
			err = idx.Set(ctx, librarian.Addr(addr), 2)
		default:
			err = idx.Set(ctx, librarian.Addr(addr), 0)
			// cryptix: not sure why this doesn't work
			// it also removes the node if this is the only follow from that peer
			// 3 state handling seems saner
			// err = idx.Delete(ctx, librarian.Addr(addr))
		}
		if err != nil {
			return errors.Wrapf(err, "db/idx contacts: failed to update index. %+v", c)
		}

		b.cachedGraph = nil
		// TODO: patch existing graph
		return nil
	}, contactsIdx)

	return b
}

func (b *builder) Authorizer(from *ssb.FeedRef, maxHops int) ssb.Authorizer {
	return &authorizer{
		b:       b,
		from:    from,
		maxHops: maxHops,
		log:     b.log,
	}
}

func (b *builder) Build() (*Graph, error) {
	dg := simple.NewWeightedDirectedGraph(0, math.Inf(1))
	known := make(key2node)

	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	if b.cachedGraph != nil {
		return b.cachedGraph, nil
	}

	err := b.kv.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		for iter.Rewind(); iter.Valid(); iter.Next() {
			it := iter.Item()
			k := it.Key()
			if len(k) < 65 {
				// fmt.Printf("skipping: %q\n", string(k))
				continue
			}

			from := ssb.FeedRef{Algo: "ed25519", ID: k[:32]}
			to := ssb.FeedRef{Algo: "ed25519", ID: k[33:]}

			if bytes.Equal(from.ID, to.ID) {
				// contact self?!
				return nil
			}

			v, err := it.Value()
			if err != nil {
				return errors.Wrap(err, "friends: couldn't get idx value")
			}

			var bfrom [32]byte
			copy(bfrom[:], from.ID)
			nFrom, has := known[bfrom]
			if !has {
				nFrom = &contactNode{dg.NewNode(), &from, ""}
				dg.AddNode(nFrom)
				known[bfrom] = nFrom
			}

			var bto [32]byte
			copy(bto[:], to.ID)
			nTo, has := known[bto]
			if !has {
				nTo = &contactNode{dg.NewNode(), &to, ""}
				dg.AddNode(nTo)
				known[bto] = nTo
			}

			w := math.Inf(-1)
			if len(v) != 1 {
				return errors.Errorf("badgerGraph: invalid state val(%v) for %s:%s", v, from.Ref(), to.Ref())
			}
			fstate := v[0]
			switch fstate {
			case '0':
				if dg.HasEdgeFromTo(nFrom.ID(), nTo.ID()) {
					dg.RemoveEdge(nFrom.ID(), nTo.ID())
				}
				return nil
			case '1':
				w = 1
			case '2': // blocking
				w = math.Inf(1)
			default:
				return errors.Errorf("badgerGraph: unhandled state val(%v) for %s:%s", fstate, from.Ref(), to.Ref())
			}

			edg := simple.WeightedEdge{F: nFrom, T: nTo, W: w}
			dg.SetWeightedEdge(contactEdge{
				WeightedEdge: edg,
				isBlock:      fstate == '2',
			})
		}
		return nil
	})

	g := &Graph{lookup: known}
	g.WeightedDirectedGraph = *dg
	b.cachedGraph = g
	return g, err
}

type Lookup struct {
	dijk   path.Shortest
	lookup key2node
}

func (l Lookup) Dist(to *ssb.FeedRef) ([]graph.Node, float64) {
	var bto [32]byte
	copy(bto[:], to.ID)
	nTo, has := l.lookup[bto]
	if !has {
		return nil, math.Inf(-1)
	}
	return l.dijk.To(nTo.ID())
}

func (b *builder) Follows(fr *ssb.FeedRef) ([]*ssb.FeedRef, error) {
	var friends []*ssb.FeedRef
	err := b.kv.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		prefix := append(fr.ID, ':')

		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			it := iter.Item()
			k := it.Key()
			c := ssb.FeedRef{
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
