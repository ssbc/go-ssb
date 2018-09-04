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

func (h *handler) pourFeed(ctx context.Context, req *muxrpc.Request) error {
	// check & parse args
	if len(req.Args) < 1 {
		return errors.New("not enough arguments, expecting feed id")
	}
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

	// h.Info.Log("event", "pour req", "ref", feedRef.Ref(), "has", latest, "want", qry.Seq)

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
		if qry.Seq >= int64(v) { // more than we got
			return errors.Wrap(req.Stream.Close(), "pour: failed to close")
		}

		userSequences, err := userLog.Query(margaret.Gte(margaret.BaseSeq(qry.Seq)), margaret.Limit(int(qry.Limit)))
		if err != nil {
			return errors.Wrapf(err, "illegal user log query seq:%d - limit:%d", qry.Seq, qry.Limit)
		}

		sent := 0
		rootLog := h.Repo.RootLog()
		wrapSink := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
			v, err := rootLog.Get(val.(margaret.Seq))
			if err != nil {
				return errors.Wrapf(err, "failed to load message %v", val)
			}
			stMsg, ok := v.(message.StoredMessage)
			if !ok {
				return errors.Errorf("wrong message type. expected %T - got %T", stMsg, v)
			}
			if err := req.Stream.Pour(ctx, message.RawSignedMessage{RawMessage: stMsg.Raw}); err != nil {
				return errors.Wrap(err, "failed to pour msg")
			}
			sent++
			return nil
		})

		if err := luigi.Pump(ctx, wrapSink, userSequences); err != nil {
			return errors.Wrap(err, "failed to pump messages to peer")
		}

		h.Info.Log("poured", ref.Ref(), "from", qry.Seq, "sent", sent)
	default:
		return errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", latest)
	}
	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}
