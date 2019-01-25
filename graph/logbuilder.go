package graph

import (
	"context"
	"encoding/json"
	"math"
	"sync"

	"go.cryptoscope.co/ssb/message"

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
	librarian.SinkIndex
	//  KILL ME
	//  KILL ME
	//  KILL ME

	logger kitlog.Logger

	log margaret.Log

	cacheLock   sync.Mutex
	cachedGraph *Graph
}

func NewLogBuilder(logger kitlog.Logger, contacts margaret.Log) (Builder, error) {
	lb := logBuilder{
		logger: logger,
		log:    contacts,
	}

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

type contactEdge struct {
	simple.WeightedEdge
	isBlock bool
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

		msg := v.(message.StoredMessage)

		var c struct {
			Author  *ssb.FeedRef
			Content ssb.Contact
		}
		err = json.Unmarshal(msg.Raw, &c)
		if err != nil {
			if ssb.IsMessageUnusable(err) {
				return nil
			}
			b.logger.Log("msg", "skipped contact message", "reason", err)
			return nil
		}

		var bfrom [32]byte
		copy(bfrom[:], c.Author.ID)
		nFrom, has := known[bfrom]
		if !has {
			nFrom = contactNode{dg.NewNode(), c.Author}
			dg.AddNode(nFrom)
			known[bfrom] = nFrom
		}

		var bto [32]byte
		copy(bto[:], c.Content.Contact.ID)
		nTo, has := known[bto]
		if !has {
			nTo = contactNode{dg.NewNode(), c.Content.Contact}
			dg.AddNode(nTo)
			known[bto] = nTo
		}

		w := math.Inf(-1)
		if c.Content.Following {
			w = 1
		} else if c.Content.Blocking {
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
			isBlock:      c.Content.Blocking,
		})

		return nil
	})
	err = luigi.Pump(context.TODO(), snk, src)
	if err != nil {
		return nil, errors.Wrap(err, "friends: couldn't get idx value")
	}
	g := &Graph{
		dg:     dg,
		lookup: known,
	}
	b.cachedGraph = g
	return g, nil
}

func (b *logBuilder) Follows(fr *ssb.FeedRef) ([]*ssb.FeedRef, error) {
	panic("TODO")
}
