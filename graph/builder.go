package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"sync"

	"github.com/dgraph-io/badger"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"

	"go.cryptoscope.co/ssb"
)

// Builder can build a trust graph and answer other questions
type Builder interface {
	librarian.SinkIndex

	Build() (*Graph, error)
	Follows(*ssb.FeedRef) (*StrFeedSet, error)
	Hops(*ssb.FeedRef, int) *StrFeedSet
	Authorizer(from *ssb.FeedRef, maxHops int) ssb.Authorizer
}

type builder struct {
	librarian.SinkIndex
	kv  *badger.DB
	idx librarian.Index
	log kitlog.Logger

	cacheLock   sync.Mutex
	cachedGraph *Graph
}

// NewBuilder creates a Builder that is backed by a badger database
func NewBuilder(log kitlog.Logger, db *badger.DB) *builder {
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

		abs, ok := val.(ssb.Message)
		if !ok {
			err := errors.Errorf("graph/idx: invalid msg value %T", val)
			log.Log("msg", "contact eval failed", "reason", err)
			return err
		}

		var c ssb.Contact
		err := json.Unmarshal(abs.ContentBytes(), &c)
		if err != nil {
			// just ignore invalid messages, nothing to do with them (unless you are debugging something)
			// err = errors.Wrapf(err, "db/idx contacts: first json unmarshal failed (msg: %v)", abs.Key().Ref())
			// log.Log("msg", "skipped contact message", "reason", err)
			return nil
		}

		addr := abs.Author().StoredAddr()
		addr += c.Contact.StoredAddr()
		switch {
		case c.Following:
			err = idx.Set(ctx, addr, 1)
		case c.Blocking:
			err = idx.Set(ctx, addr, 2)
		default:
			err = idx.Set(ctx, addr, 0)
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
			if len(k) != 66 {
				continue
			}

			rawFrom := k[:33]
			rawTo := k[33:]

			if bytes.Equal(rawFrom, rawTo) {
				// contact self?!
				continue
			}

			var to, from ssb.StorageRef
			if err := from.Unmarshal(rawFrom); err != nil {
				return errors.Wrapf(err, "builder: couldnt idx key value (from)")
			}
			if err := to.Unmarshal(rawTo); err != nil {
				return errors.Wrap(err, "builder: couldnt idx key value (to)")
			}

			bfrom := librarian.Addr(rawFrom)
			nFrom, has := known[bfrom]
			if !has {
				nFrom = &contactNode{dg.NewNode(), &from, ""}
				dg.AddNode(nFrom)
				known[bfrom] = nFrom
			}

			bto := librarian.Addr(rawTo)
			nTo, has := known[bto]
			if !has {
				nTo = &contactNode{dg.NewNode(), &to, ""}
				dg.AddNode(nTo)
				known[bto] = nTo
			}

			if nFrom.ID() == nTo.ID() {
				continue
			}

			w := math.Inf(-1)
			err := it.Value(func(v []byte) error {
				if len(v) >= 1 {
					switch v[0] {
					case '0': // not following
					case '1':
						w = 1
					case '2':
						w = math.Inf(1)
					default:
						return errors.Errorf("barbage value in graph strore")
					}
				}
				return nil
			})
			if err != nil {
				return errors.Wrap(err, "failed to get value from item")
			}

			if math.IsInf(w, -1) {
				continue
			}

			edg := simple.WeightedEdge{F: nFrom, T: nTo, W: w}
			dg.SetWeightedEdge(contactEdge{
				WeightedEdge: edg,
				isBlock:      math.IsInf(w, 1),
			})
		}
		return nil
	})

	g := &Graph{lookup: known}
	g.WeightedDirectedGraph = dg
	b.cachedGraph = g
	return g, err
}

type Lookup struct {
	dijk   path.Shortest
	lookup key2node
}

func (l Lookup) Dist(to *ssb.FeedRef) ([]graph.Node, float64) {
	bto := to.StoredAddr()
	nTo, has := l.lookup[bto]
	if !has {
		return nil, math.Inf(-1)
	}
	return l.dijk.To(nTo.ID())
}

func (b *builder) Follows(forRef *ssb.FeedRef) (*StrFeedSet, error) {
	if forRef == nil {
		panic("nil feed ref")
	}
	fs := NewFeedSet(50)
	err := b.kv.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		prefix := []byte(forRef.StoredAddr())
		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			it := iter.Item()
			k := it.Key()

			err := it.Value(func(v []byte) error {
				if len(v) >= 1 && v[0] == '1' {
					// extract 2nd feed ref out of db key
					// TODO: use compact StoredAddr
					var sr ssb.StorageRef
					err := sr.Unmarshal(k[33:])
					if err != nil {
						return errors.Wrapf(err, "follows(%s): invalid ref entry in db for feed", forRef.Ref())
					}
					if err := fs.AddStored(&sr); err != nil {
						return errors.Wrapf(err, "follows(%s): couldn't add parsed ref feed", forRef.Ref())
					}
				}
				return nil
			})
			if err != nil {
				return errors.Wrap(err, "failed to get value from iter")
			}
		}
		return nil
	})
	return fs, err
}

// Hops returns a slice of feed refrences that are in a particulare range of from
// max == 0: only direct follows of from
// max == 1: max:0 + follows of friends of from
// max == 2: max:1 + follows of their friends
func (b *builder) Hops(from *ssb.FeedRef, max int) *StrFeedSet {
	max++
	walked := NewFeedSet(1000)
	err := walked.AddRef(from)
	if err != nil {
		b.log.Log("event", "error", "msg", "add failed", "err", err)
		return nil
	}
	err = b.recurseHops(walked, from, max)
	if err != nil {
		b.log.Log("event", "error", "msg", "recurse failed", "err", err)
		return nil
	}
	return walked
}

func (b *builder) recurseHops(walked *StrFeedSet, from *ssb.FeedRef, depth int) error {
	// b.log.Log("recursing", from.Ref(), "d", depth)
	if depth == 0 {
		return nil
	}

	fromFollows, err := b.Follows(from)
	if err != nil {
		return errors.Wrapf(err, "recurseHops(%d): from follow listing failed", depth)
	}

	followLst, err := fromFollows.List()
	if err != nil {
		return errors.Wrapf(err, "recurseHops(%d): invalid entry in feed set", depth)
	}

	for i, followedByFrom := range followLst {
		err := walked.AddRef(followedByFrom)
		if err != nil {
			return errors.Wrapf(err, "recurseHops(%d): add list entry(%d) failed", depth, i)
		}

		dstFollows, err := b.Follows(followedByFrom)
		if err != nil {
			return errors.Wrapf(err, "recurseHops(%d): follows from entry(%d) failed", depth, i)
		}

		isF := dstFollows.Has(from)
		if isF { // found a friend, recurse
			if err := b.recurseHops(walked, followedByFrom, depth-1); err != nil {
				return err
			}
		}
		// b.log.Log("depth", depth, "from", from.Ref()[1:5], "follows", followedByFrom.Ref()[1:5], "friend", isF, "cnt", dstFollows.Count())
	}
	return nil
}
