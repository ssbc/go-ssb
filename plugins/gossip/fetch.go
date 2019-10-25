// SPDX-License-Identifier: MIT

package gossip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/codec"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/message"
)

type ErrWrongSequence struct {
	Ref             *ssb.FeedRef
	Indexed, Stored margaret.Seq
}

func (e ErrWrongSequence) Error() string {
	return fmt.Sprintf("consistency error: wrong stored message sequence for feed %s. stored:%d indexed:%d",
		e.Ref.Ref(), e.Stored, e.Indexed)
}

func (h *handler) fetchAllLib(
	ctx context.Context,
	e muxrpc.Endpoint,
	lst []librarian.Addr,
) error {
	var refs = graph.NewFeedSet(len(lst))
	for i, addr := range lst {
		var sr ssb.StorageRef
		err := sr.Unmarshal([]byte(addr))
		if err != nil {
			return errors.Wrapf(err, "fetchLib(%d) failed to parse (%q)", i, addr)
		}
		if err := refs.AddStored(&sr); err != nil {
			return errors.Wrapf(err, "fetchLib(%d) set add failed", i)
		}
	}
	return h.fetchAll(ctx, e, refs)
}

func (h *handler) fetchAllMinus(
	ctx context.Context,
	e muxrpc.Endpoint,
	fs *graph.StrFeedSet,
	got []librarian.Addr,
) error {
	lst, err := fs.List()
	if err != nil {
		return err
	}
	var refs = graph.NewFeedSet(len(lst))
	for _, ref := range lst {
		if !isIn(got, ref) {
			err := refs.AddRef(ref)
			if err != nil {
				return err
			}
		}
	}
	return h.fetchAll(ctx, e, refs)
}

func (h *handler) fetchAll(
	ctx context.Context,
	e muxrpc.Endpoint,
	fs *graph.StrFeedSet,
) error {
	// we don't just want them all parallel right nw
	// this kind of concurrency is way to harsh on the runtime
	// we need some kind of FeedManager, similar to Blobs
	// which we can ask for which feeds aren't in transit,
	// due for a (probabilistic) update
	// and manage live feeds more granularly across open connections

	lst, err := fs.List()
	if err != nil {
		return err
	}
	for _, r := range lst {
		err := h.fetchFeed(ctx, r, e)
		if muxrpc.IsSinkClosed(err) || errors.Cause(err) == context.Canceled || errors.Cause(err) == muxrpc.ErrSessionTerminated {
			return err
		} else if err != nil {
			// assuming forked feed for instance
			h.Info.Log("msg", "fetchFeed stored failed", "err", err)
		}
	}
	return nil
}

func isIn(list []librarian.Addr, a *ssb.FeedRef) bool {
	for _, el := range list {
		if bytes.Equal([]byte(a.StoredAddr()), []byte(el)) {
			return true
		}
	}
	return false
}

// fetchFeed requests the feed fr from endpoint e into the repo of the handler
func (g *handler) fetchFeed(
	ctx context.Context,
	fr *ssb.FeedRef,
	edp muxrpc.Endpoint,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	// check our latest
	addr := fr.StoredAddr()
	g.activeLock.Lock()
	_, ok := g.activeFetch.Load(addr)
	if ok {
		// errors.Errorf("fetchFeed: crawl of %x active", addr[:5])
		g.activeLock.Unlock()
		return nil
	}
	if g.sysGauge != nil {
		g.sysGauge.With("part", "fetches").Add(1)
	}
	g.activeFetch.Store(addr, true)
	g.activeLock.Unlock()
	defer func() {
		g.activeLock.Lock()
		g.activeFetch.Delete(addr)
		g.activeLock.Unlock()
		if g.sysGauge != nil {
			g.sysGauge.With("part", "fetches").Add(-1)
		}
	}()
	userLog, err := g.UserFeeds.Get(addr)
	if err != nil {
		return errors.Wrapf(err, "failed to open sublog for user")
	}
	latest, err := userLog.Seq().Value()
	if err != nil {
		return errors.Wrapf(err, "failed to observe latest")
	}
	var (
		latestSeq margaret.BaseSeq
		latestMsg ssb.Message
	)
	switch v := latest.(type) {
	case librarian.UnsetValue:
		// nothing stored, fetch from zero
	case margaret.BaseSeq:
		latestSeq = v + 1 // sublog is 0-init while ssb chains start at 1
		if v >= 0 {
			rootLogValue, err := userLog.Get(v)
			if err != nil {
				return errors.Wrapf(err, "failed to look up root seq for latest user sublog")
			}
			msgV, err := g.RootLog.Get(rootLogValue.(margaret.Seq))
			if err != nil {
				return errors.Wrapf(err, "failed retreive stored message")
			}

			abs, ok := msgV.(ssb.Message)
			if !ok {
				return errors.Errorf("fetch: wrong message type. expected %T - got %T", latestMsg, msgV)
			}

			latestMsg = abs

			if hasSeq := latestMsg.Seq(); hasSeq != latestSeq.Seq() {
				return &ErrWrongSequence{Stored: latestMsg, Indexed: latestSeq, Ref: fr}
			}
		}
	}

	startSeq := latestSeq
	info := log.With(g.Info, "fr", fr.Ref(), "latest", startSeq) //, "me", g.Id.Ref())

	var q = message.CreateHistArgs{
		ID:         fr.Ref(),
		Seq:        int64(latestSeq + 1),
		StreamArgs: message.StreamArgs{Limit: -1},
	}
	start := time.Now()

	toLong, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer func() {
		cancel()
		if n := latestSeq - startSeq; n > 0 {
			if g.sysGauge != nil {
				g.sysGauge.With("part", "msgs").Add(float64(n))
			}
			if g.sysCtr != nil {
				g.sysCtr.With("event", "gossiprx").Add(float64(n))
			} else {
				level.Debug(info).Log("event", "gossiprx", "new", n, "took", time.Since(start))
			}
		}
	}()

	method := muxrpc.Method{"createHistoryStream"}

	store := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}
		_, err = g.RootLog.Append(val)
		return errors.Wrap(err, "failed to append verified message to rootLog")
	})

	var (
		src luigi.Source
		snk luigi.Sink = message.NewVerifySink(fr, latestSeq, latestMsg, store, g.hmacSec)
	)

	switch fr.Algo {
	case ssb.RefAlgoFeedSSB1:
		src, err = edp.Source(toLong, json.RawMessage{}, method, q)
	case ssb.RefAlgoFeedGabby:
		src, err = edp.Source(toLong, codec.Body{}, method, q)
	}
	if err != nil {
		return errors.Wrapf(err, "fetchFeed(%s:%d) failed to create source", fr.Ref(), latestSeq)
	}

	// count the received messages
	snk = mfr.SinkMap(snk, func(_ context.Context, val interface{}) (interface{}, error) {
		latestSeq++
		return val, nil
	})

	// info.Log("starting", "fetch")
	err = luigi.Pump(toLong, snk, src)
	return errors.Wrap(err, "gossip pump failed")
}
