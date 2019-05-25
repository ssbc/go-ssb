package graph

import (
	"context"
	"encoding/json"
	"math"
	"sync"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"gonum.org/v1/gonum/graph/simple"
)

type logBuilder struct {
	//  KILL ME
	//  KILL ME
	// this is just a left-over from the badger-based builder
	// it's only here to fulfil the Builder interface
	// badger _should_ split it's indexing out of it and then we can remove this here as well
	librarian.SinkIndex
	//  KILL ME
	// dont! call these methods
	//  KILL ME
	//  KILL ME

	logger kitlog.Logger

	log margaret.Log

	cacheLock   sync.Mutex
	cachedGraph *Graph
}

// NewLogBuilder is a much nicer abstraction than the direct k:v implementation.
// most likely terribly slow though. Additionally, we have to unmarshal from stored.Raw again...
// TODO: actually compare the two with benchmarks if only to compare the 3rd!
func NewLogBuilder(logger kitlog.Logger, contacts margaret.Log) (Builder, error) {
	lb := logBuilder{
		logger: logger,
		log:    contacts,
	}

	fsnk := luigi.FuncSink(func(ctx context.Context, v interface{}, closeErr error) error {
		if closeErr != nil {
			return closeErr
		}
		logger.Log("msg", "new contact invalidating graph - debounce?")
		lb.cacheLock.Lock()
		lb.cachedGraph = nil
		lb.cacheLock.Unlock()
		return nil
	})
	contacts.Seq().Register(fsnk)

	return &lb, nil
}

func (b *logBuilder) Authorizer(from *ssb.FeedRef, maxHops int) ssb.Authorizer {
	return &authorizer{
		b:       b,
		from:    from,
		maxHops: maxHops,
		log:     b.logger,
	}
}

func (b *logBuilder) Build() (*Graph, error) {
	dg := simple.NewWeightedDirectedGraph(0, math.Inf(1))
	known := make(key2node)

	b.cacheLock.Lock()
	defer b.cacheLock.Unlock()

	if b.cachedGraph != nil {
		return b.cachedGraph, nil
	}

	src, err := b.log.Query()
	if err != nil {
		return nil, errors.Wrap(err, "friends: couldn't get idx value")
	}

	snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}

		abs, ok := v.(ssb.Message)
		if !ok {
			err := errors.Errorf("graph/idx: invalid msg value %T", v)
			return err
		}

		var c ssb.Contact
		err = json.Unmarshal(abs.ContentBytes(), &c)
		if err != nil {
			err = errors.Wrapf(err, "db/idx contacts: first json unmarshal failed (msg: %s)", abs.Key().Ref())
			return nil
		}

		author := abs.Author()
		contact := c.Contact

		if author.Equal(contact) {
			// contact self?!
			return nil
		}

		bfrom := author.StoredAddr()
		nFrom, has := known[bfrom]
		if !has {
			nFrom = &contactNode{dg.NewNode(), author, ""}
			dg.AddNode(nFrom)
			known[bfrom] = nFrom
		}

		bto := contact.StoredAddr()
		nTo, has := known[bto]
		if !has {
			nTo = &contactNode{dg.NewNode(), contact, ""}
			dg.AddNode(nTo)
			known[bto] = nTo
		}

		w := math.Inf(-1)
		if c.Following {
			w = 1
		} else if c.Blocking {
			w = math.Inf(1)
		} else {
			if dg.HasEdgeFromTo(nFrom.ID(), nTo.ID()) {
				dg.RemoveEdge(nFrom.ID(), nTo.ID())
			}
			return nil
		}

		edg := simple.WeightedEdge{F: nFrom, T: nTo, W: w}
		dg.SetWeightedEdge(contactEdge{
			WeightedEdge: edg,
			isBlock:      c.Blocking,
		})
		return nil
	})

	err = luigi.Pump(context.TODO(), snk, src)
	if err != nil {
		return nil, errors.Wrap(err, "friends: couldn't get idx value")
	}
	g := &Graph{
		WeightedDirectedGraph: *dg,
		lookup:                known,
	}
	b.cachedGraph = g
	return g, nil
}

func (b *logBuilder) Follows(from *ssb.FeedRef) (FeedSet, error) {
	g, err := b.Build()
	if err != nil {
		return nil, errors.Wrap(err, "follows: couldn't build graph")
	}

	fb := from.StoredAddr()
	nFrom, has := g.lookup[fb]
	if !has {
		return nil, ErrNoSuchFrom{from}
	}

	nodes := g.From(nFrom.ID())

	refs := NewFeedSet(nodes.Len())

	for nodes.Next() {
		cnv := nodes.Node().(contactNode)
		// warning - ignores edge type!
		edg := g.Edge(nFrom.ID(), cnv.ID())
		if edg.(contactEdge).Weight() == 1 {
			if err := refs.AddRef(cnv.feed); err != nil {
				return nil, err
			}
		}
	}
	return refs, nil
}

func (b *logBuilder) Hops(from *ssb.FeedRef, max int) FeedSet {
	// it would be terrible to do this without some kind of filtering/caching
	panic("TODO:unsupported")
}
