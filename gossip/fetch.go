package gossip

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/message"
)

func (g *Handler) fetchFeed(ctx context.Context, fr sbot.FeedRef, e muxrpc.Endpoint) error {
	latestIdxKey := librarian.Addr(fmt.Sprintf("latest:%s", fr.Ref()))
	idx := g.Repo.GossipIndex()
	latestObv, err := idx.Get(ctx, latestIdxKey)
	if err != nil {
		return errors.Wrapf(err, "idx latest failed")
	}
	latest, err := latestObv.Value()
	if err != nil {
		return errors.Wrapf(err, "failed to observe latest")
	}

	var latestSeq margaret.Seq
	switch v := latest.(type) {
	case librarian.UnsetValue:
	case margaret.Seq:
		latestSeq = v
	}
	// me := g.Repo.KeyPair()
	startSeq := latestSeq
	info := log.With(g.Info, "remote", fr.Ref(), "latest", startSeq) //, "me", me.Id.Ref())

	var q = message.CreateHistArgs{
		Keys:  false,
		Live:  false,
		Id:    fr.Ref(),
		Seq:   latestSeq + 1,
		Limit: 1000,
	}
	start := time.Now()
	source, err := e.Source(ctx, message.RawSignedMessage{}, []string{"createHistoryStream"}, q)
	if err != nil {
		return errors.Wrapf(err, "fetchFeed(%s:%d) failed to create source", fr.Ref(), latestSeq)
	}
	// info.Log("event", "start sync")
	var more bool
	for {
		v, err := source.Next(ctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s:%d): failed to drain", fr.Ref(), latestSeq)
		}
		rmsg := v.(message.RawSignedMessage)

		ref, dmsg, err := message.Verify(rmsg.RawMessage)
		if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s:%d): simple Encode failed", fr.Ref(), latestSeq)
		}
		//info.Log("dbg", "got message", "seq", dmsg.Sequence)

		// todo: check previous etc.. maybe we want a mapping sink here
		_, err = g.Repo.Log().Append(message.StoredMessage{
			Author:    dmsg.Author,
			Previous:  dmsg.Previous,
			Key:       *ref,
			Sequence:  dmsg.Sequence,
			Timestamp: time.Now(),
			Raw:       rmsg.RawMessage,
		})
		if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s): failed to append message(%s:%d)", fr.Ref(), ref.Ref(), dmsg.Sequence)
		}
		more = true
		latestSeq = dmsg.Sequence
	} // hist drained

	if !more {
		// info.Log("event", "up2date", "took", time.Since(start))
		return nil
	}

	if err := idx.Set(ctx, latestIdxKey, latestSeq); err != nil {
		return errors.Wrapf(err, "fetchFeed(%s): failed to update sequence %d", fr.Ref(), latestSeq)
	}

	f := func(ctx context.Context, seq margaret.Seq, v interface{}, idx librarian.SetterIndex) error {
		smsg, ok := v.(message.StoredMessage)
		if !ok {
			return errors.Errorf("fetchFeed(%s): unexpected type: %T - wanted storedMsg", fr.Ref(), v)
		}
		addr := fmt.Sprintf("%s:%06d", smsg.Author.Ref(), smsg.Sequence)
		err := idx.Set(ctx, librarian.Addr(addr), seq)
		return errors.Wrapf(err, "fetchFeed(%s): failed to update idx for k:%s - v:%d", fr.Ref(), addr, seq)
	}
	sinkIdx := librarian.NewSinkIndex(f, idx)

	src, err := g.Repo.Log().Query(sinkIdx.QuerySpec())
	if err != nil {
		return errors.Wrapf(err, "fetchFeed(%s): failed to construct index update query", fr.Ref())
	}
	if err := luigi.Pump(ctx, sinkIdx, src); err != nil {
		return errors.Wrapf(err, "fetchFeed(%s): error pumping from queried src to SinkIndex", fr.Ref())
	}

	info.Log("event", "verfied2updated", "new", latestSeq-startSeq, "took", time.Since(start))
	return nil
}
