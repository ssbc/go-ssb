package gossip

import (
	"bytes"
	"context"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

// fetchFeed requests the feed fr from endpoint e into the repo of the handler
func (g *handler) fetchFeed(ctx context.Context, fr *ssb.FeedRef, edp muxrpc.Endpoint) error {
	// check our latest
	addr := librarian.Addr(fr.ID)
	_, ok := g.activeFetch.Load(addr)
	if ok {
		// errors.Errorf("fetchFeed: crawl of %x active", addr[:5])
		return nil
	}
	if g.sysGauge != nil {
		g.sysGauge.With("part", "fetches").Add(1)
	}
	g.activeFetch.Store(addr, true)
	defer func() {
		g.activeFetch.Delete(addr)
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
				return errors.Errorf("wrong stored message sequence. stored:%d indexed:%d", latestMsg.Sequence, latestSeq)
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
	crawlCtx, crawlCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer crawlCancel()
	source, err := edp.Source(crawlCtx, message.RawSignedMessage{}, []string{"createHistoryStream"}, q)
	if err != nil {
		return errors.Wrapf(err, "fetchFeed(%s:%d) failed to create source", fr.Ref(), latestSeq)
	}
	// info.Log("debug", "called createHistoryStream", "qry", fmt.Sprintf("%v", q))

	for {
		v, err := source.Next(crawlCtx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s:%d): failed to drain", fr.Ref(), latestSeq)
		}

		rmsg := v.(message.RawSignedMessage)
		ref, dmsg, err := message.Verify(rmsg.RawMessage)
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

	if n := latestSeq - startSeq; n > 0 {
		info.Log("event", "fetchFeed", "new", n, "took", time.Since(start))
		if g.sysGauge != nil {
			g.sysGauge.With("part", "msgs").Add(float64(n))
		}
		if g.sysCtr != nil {
			g.sysCtr.With("event", "gossiprx").Add(float64(n))
		}
	}
	return nil
}
