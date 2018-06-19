package gossip

import (
	"context"
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

// fetchFeed requests the feed fr from endpoint e into the repo of the handler
func (g *Handler) fetchFeed(ctx context.Context, fr sbot.FeedRef, e muxrpc.Endpoint) error {
	// check our latest
	userLog, err := g.Repo.UserFeeds().Get(librarian.Addr(fr.ID))
	if err != nil {
		return errors.Wrapf(err, "failed to open sublog for user")
	}
	latest, err := userLog.Seq().Value()
	if err != nil {
		return errors.Wrapf(err, "failed to observe latest")
	}
	var latestSeq margaret.BaseSeq
	switch v := latest.(type) {
	case librarian.UnsetValue:
	case margaret.BaseSeq:
		latestSeq = v + 1 // sublog is 0-init while ssb chains start at 1
	}

	// me := g.Repo.KeyPair()
	startSeq := latestSeq
	info := log.With(g.Info, "remote", fr.Ref(), "latest", startSeq) //, "me", me.Id.Ref())

	var q = message.CreateHistArgs{
		Keys:  false,
		Live:  false,
		Id:    fr.Ref(),
		Seq:   int64(latestSeq + 1),
		Limit: 1000,
	}
	start := time.Now()
	source, err := e.Source(ctx, message.RawSignedMessage{}, []string{"createHistoryStream"}, q)
	if err != nil {
		return errors.Wrapf(err, "fetchFeed(%s:%d) failed to create source", fr.Ref(), latestSeq)
	}
	// info.Log("debug", "start sync")

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
		// info.Log("debug", "got message", "seq", dmsg.Sequence)

		// todo: check previous etc.. maybe we want a mapping sink here
		_, err = g.Repo.RootLog().Append(message.StoredMessage{
			Author:    &dmsg.Author,
			Previous:  &dmsg.Previous,
			Key:       ref,
			Sequence:  dmsg.Sequence,
			Timestamp: time.Now(),
			Raw:       rmsg.RawMessage,
		})
		if err != nil {
			return errors.Wrapf(err, "fetchFeed(%s): failed to append message(%s:%d)", fr.Ref(), ref.Ref(), dmsg.Sequence)
		}
		latestSeq = dmsg.Sequence
	} // hist drained

	info.Log("event", "verfied2updated", "new", latestSeq-startSeq, "took", time.Since(start))
	return nil
}
