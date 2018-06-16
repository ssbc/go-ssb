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
	me := g.Repo.KeyPair()
	info := log.With(g.Info, "remote", fr.Ref(), "me", me.Id.Ref())
	info.Log("dbg", "fetch", "latest", latestSeq)

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
		return errors.Wrapf(err, "createHistoryStream failed")
	}

	var more bool
	for {
		v, err := source.Next(ctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "failed to drain createHistoryStream logSeq:%d", latestSeq)
		}
		rmsg := v.(message.RawSignedMessage)

		ref, dmsg, err := message.Verify(rmsg.RawMessage)
		if err != nil {
			return errors.Wrap(err, "simple Encode failed")
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
			return errors.Wrapf(err, "failed to append message (%s) Seq:%d", ref.Ref(), dmsg.Sequence)
		}
		more = true
		latestSeq = dmsg.Sequence
	} // hist drained

	if !more {
		return nil
	}

	if err := idx.Set(ctx, latestIdxKey, latestSeq); err != nil {
		return errors.Wrapf(err, "failed to update sequence for author %s", fr.Ref())
	}

	f := func(ctx context.Context, seq margaret.Seq, v interface{}, idx librarian.SetterIndex) error {
		smsg, ok := v.(message.StoredMessage)
		if !ok {
			return errors.Errorf("unexpected type: %T - wanted storedMsg", v)
		}
		addr := fmt.Sprintf("%s:%06d", smsg.Author.Ref(), smsg.Sequence)
		err := idx.Set(ctx, librarian.Addr(addr), seq)
		return errors.Wrapf(err, "failed to update idx for k:%s - v:%d", addr, seq)
	}
	sinkIdx := librarian.NewSinkIndex(f, idx)

	src, err := g.Repo.Log().Query(sinkIdx.QuerySpec())
	if err != nil {
		return errors.Wrapf(err, "failed to construct index update query")
	}
	if err := luigi.Pump(ctx, sinkIdx, src); err != nil {
		return errors.Wrap(err, "error pumping from queried src to SinkIndex")
	}

	info.Log("event", "verfied2updated", "latest", latestSeq, "took", time.Since(start))
	return nil
}
