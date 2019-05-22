package gossip

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
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

func (h *handler) fetchAllLib(ctx context.Context, e muxrpc.Endpoint, lst []librarian.Addr) error {
	var refs = graph.NewFeedSet(len(lst))
	for _, addr := range lst {
		err := refs.AddB([]byte(addr))
		if err != nil {
			return err
		}
	}
	return h.fetchAll(ctx, e, refs)
}

func (h *handler) fetchAllMinus(ctx context.Context, e muxrpc.Endpoint, fs graph.FeedSet, got []librarian.Addr) error {
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

func (h *handler) fetchAll(ctx context.Context, e muxrpc.Endpoint, fs graph.FeedSet) error {
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
		if muxrpc.IsSinkClosed(err) || errors.Cause(err) == context.Canceled {
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
		if bytes.Compare([]byte(el), a.ID) == 0 {
			return true
		}
	}
	return false
}

// fetchFeed requests the feed fr from endpoint e into the repo of the handler
func (g *handler) fetchFeed(ctx context.Context, fr *ssb.FeedRef, edp muxrpc.Endpoint) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	// check our latest
	addr := librarian.Addr(fr.ID)
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
		latestMsg message.StoredMessage
	)
	switch v := latest.(type) {
	case librarian.UnsetValue:
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
			var ok bool
			latestMsg, ok = msgV.(message.StoredMessage)
			if !ok {
				return errors.Errorf("wrong message type. expected %T - got %T", latestMsg, v)
			}

			if latestMsg.Sequence != latestSeq {
				return &ErrWrongSequence{Stored: latestMsg.Sequence, Indexed: latestSeq, Ref: fr}
			}
		}
	}

	startSeq := latestSeq
	info := log.With(g.Info, "fr", fr.Ref(), "latest", startSeq) // "me", g.Id.Ref(), "from", ...)

	var q = message.CreateHistArgs{
		Id:    fr.Ref(),
		Seq:   int64(latestSeq + 1),
		Limit: -1,
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
			}
			info.Log("event", "gossiprx", "new", n, "took", time.Since(start))
		}
	}()

	source, err := edp.Source(toLong, message.RawSignedMessage{}, []string{"createHistoryStream"}, q)
	if err != nil {
		return errors.Wrapf(err, "fetchFeed(%s:%d) failed to create source", fr.Ref(), latestSeq)
	}
	// info.Log("debug", "called createHistoryStream", "qry", fmt.Sprintf("%v", q))

	for {
		v, err := source.Next(toLong)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s:%d): failed to drain", fr.Ref(), latestSeq)
		}

		rmsg := v.(message.RawSignedMessage)
		ref, dmsg, err := message.Verify(rmsg.RawMessage, g.hmacSec)
		if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s:%d): message verify failed", fr.Ref(), latestSeq)
		}

		if latestSeq > 1 {
			if bytes.Compare(latestMsg.Key.Hash, dmsg.Previous.Hash) != 0 {
				return errors.Errorf("fetchFeed(%s:%d): previous compare failed expected:%s incoming:%s",
					fr.Ref(),
					latestSeq,
					latestMsg.Key.Ref(),
					dmsg.Previous.Ref(),
				)
			}
			if latestMsg.Sequence+1 != dmsg.Sequence {
				return errors.Errorf("fetchFeed(%s:%d): next.seq != curr.seq+1", fr.Ref(), latestSeq)
			}
		}

		nextMsg := message.StoredMessage{
			Author:    &dmsg.Author,
			Previous:  &dmsg.Previous,
			Key:       ref,
			Sequence:  dmsg.Sequence,
			Timestamp: time.Now(),
			Raw:       rmsg.RawMessage,
		}

		_, err = g.RootLog.Append(nextMsg)
		if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s): failed to append message(%s:%d)", fr.Ref(), ref.Ref(), dmsg.Sequence)
		}

		latestSeq = dmsg.Sequence
		latestMsg = nextMsg
	} // hist drained

	return nil
}
