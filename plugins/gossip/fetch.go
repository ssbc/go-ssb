// SPDX-License-Identifier: MIT

package gossip

import (
	"bytes"
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/codec"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/luigiutils"
	"go.cryptoscope.co/ssb/internal/neterr"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message"
)

func (h *handler) fetchAll(
	ctx context.Context,
	e muxrpc.Endpoint,
	set *ssb.StrFeedSet,
) error {
	lst, err := set.List()
	if err != nil {
		return err
	}
	// we don't just want them all parallel right nw
	// this kind of concurrency is way to harsh on the runtime
	// we need some kind of FeedManager, similar to Blobs
	// which we can ask for which feeds aren't in transit,
	// due for a (probabilistic) update
	// and manage live feeds more granularly across open connections

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fetchGroup, ctx := errgroup.WithContext(ctx)
	work := make(chan *refs.FeedRef)

	n := 1 + (len(lst) / 10)
	// this doesnt pipeline super well
	// we can do more then one feed per core
	maxWorker := runtime.NumCPU() * 4
	if n > maxWorker { // n = max(n,maxWorker)
		n = maxWorker
	}
	for i := n; i > 0; i-- {
		fetchGroup.Go(h.makeWorker(work, ctx, e))
	}

	for _, r := range lst {
		select {
		case <-ctx.Done():
			close(work)
			fetchGroup.Wait()
			return ctx.Err()
		case work <- r:
		}
	}
	close(work)
	level.Debug(h.Info).Log("event", "feed fetch workers filled", "n", n)
	err = fetchGroup.Wait()
	level.Debug(h.Info).Log("event", "workers done", "err", err)
	return err
}

func (h *handler) makeWorker(work <-chan *refs.FeedRef, ctx context.Context, edp muxrpc.Endpoint) func() error {
	started := time.Now()
	return func() error {
		for ref := range work {
			err := h.fetchFeed(ctx, ref, edp, started)
			causeErr := errors.Cause(err)
			if muxrpc.IsSinkClosed(err) || causeErr == context.Canceled || causeErr == muxrpc.ErrSessionTerminated || neterr.IsConnBrokenErr(causeErr) {
				return err
			} else if err != nil {
				// just logging the error assuming forked feed for instance
				level.Warn(h.Info).Log("event", "skipped updating of stored feed", "err", err, "fr", ref.ShortRef())
			}
		}
		return nil
	}
}

func isIn(list []librarian.Addr, a *refs.FeedRef) bool {
	for _, el := range list {
		if bytes.Equal([]byte(storedrefs.Feed(a)), []byte(el)) {
			return true
		}
	}
	return false
}

// fetchFeed requests the feed fr from endpoint e into the repo of the handler
func (g *handler) fetchFeed(
	ctx context.Context,
	fr *refs.FeedRef,
	edp muxrpc.Endpoint,
	started time.Time,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	snk, err := g.verifySinks.GetSink(fr)
	if err != nil {
		return errors.Wrapf(err, "failed to get verify sink for feed")
	}
	var latestSeq = int(snk.Seq())
	startSeq := latestSeq
	info := log.With(g.Info, "event", "gossiprx",
		"fr", fr.ShortRef(),
		"starting", latestSeq) // , "me", g.Id.ShortRef())

	var q = message.CreateHistArgs{
		ID:         fr,
		Seq:        int64(latestSeq + 1),
		StreamArgs: message.StreamArgs{Limit: -1},
	}
	q.Live = true

	defer func() {
		if n := latestSeq - startSeq; n > 0 {
			if g.sysGauge != nil {
				g.sysGauge.With("part", "msgs").Add(float64(n))
			}
			if g.sysCtr != nil {
				g.sysCtr.With("event", "gossiprx").Add(float64(n))
			}
			level.Debug(info).Log("received", n, "took", time.Since(started))
		}
	}()

	method := muxrpc.Method{"createHistoryStream"}

	var src luigi.Source
	switch fr.Algo {
	case refs.RefAlgoFeedSSB1:
		src, err = edp.Source(ctx, json.RawMessage{}, method, q)
	case refs.RefAlgoFeedGabby:
		src, err = edp.Source(ctx, codec.Body{}, method, q)
	}
	if err != nil {
		return errors.Wrapf(err, "fetchFeed(%s:%d) failed to create source", fr.Ref(), latestSeq)
	}

	// level.Info(info).Log("starting", "fetch")
	err = luigi.Pump(ctx, luigiutils.NewSinkCounter(&latestSeq, snk), src)
	return errors.Wrap(err, "gossip pump failed")
}
