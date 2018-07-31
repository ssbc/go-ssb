package gossip

import (
	"context"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/message"
)

func (h *Handler) pourFeed(ctx context.Context, req *muxrpc.Request) error {
	// check & parse args
	qv := req.Args[0].(map[string]interface{})
	var qry message.CreateHistArgs
	if err := mapstructure.Decode(qv, &qry); err != nil {
		return errors.Wrap(err, "failed#2 to decode qry map")
	}
	ref, err := sbot.ParseRef(qry.Id)
	if err != nil {
		return errors.Wrapf(err, "illegal ref: %s", qry.Id)
	}
	feedRef, ok := ref.(*sbot.FeedRef)
	if !ok {
		return errors.Wrapf(err, "illegal ref type: %s", qry.Id)
	}

	// check what we got
	userLog, err := h.Repo.UserFeeds().Get(librarian.Addr(feedRef.ID))
	if err != nil {
		return errors.Wrapf(err, "failed to open sublog for user")
	}
	latest, err := userLog.Seq().Value()
	if err != nil {
		return errors.Wrapf(err, "failed to observe latest")
	}

	// act accordingly
	switch v := latest.(type) {
	case librarian.UnsetValue: // don't have the feed - nothing to do?
		// if h.Promisc {
		// 	err := gossipIdx.Set(ctx, latestKey, margaret.BaseSeq(0))
		// 	if err != nil {
		// 		return errors.Wrap(err, "failed to set unknown key")
		// 	}
		// }
	case margaret.BaseSeq:
		if qry.Seq >= v { // more than we got
			return errors.Wrap(req.Stream.Close(), "failed to close")
		}

		seqs, err := drainAllSeqs(ctx, userLog)
		if err != nil {
			return errors.Wrap(err, "failed to load sequences in rootLog")
		}

		rest := seqs[qry.Seq-1:]
		if len(rest) > 150 { // batch - slow but steady
			rest = rest[:150]
		}
		log := h.Repo.RootLog()
		for i, rSeq := range rest {
			v, err := log.Get(rSeq)
			if err != nil {
				return errors.Wrapf(err, "load message %d", i)
			}
			stMsg, ok := v.(*message.StoredMessage)
			if !ok {
				return errors.Errorf("wrong message type. expected *message.StoredMessage - got %T", latest)
			}
			if err := req.Stream.Pour(ctx, message.RawSignedMessage{RawMessage: stMsg.Raw}); err != nil {
				return errors.Wrap(err, "failed to pour msg to remote peer")

			}
		}
		h.Info.Log("sent", ref.Ref(), "from", qry.Seq, "rest", len(rest))
	default:
		return errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", latest)
	}
	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}

func drainAllSeqs(ctx context.Context, log margaret.Log) ([]margaret.Seq, error) {
	src, err := log.Query()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create luigi src")
	}

	var seqs []margaret.Seq
	for {
		v, err := src.Next(ctx)
		if luigi.IsEOS(err) {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to create luigi src")
		}
		s, ok := v.(margaret.BaseSeq)
		if !ok {
			return nil, errors.Errorf("wrong value type in index. expected BaseSeq - got %T", v)
		}

		seqs = append(seqs, s)
	}
	return seqs, nil
}
