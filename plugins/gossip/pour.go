package gossip

import (
	"context"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

func (h *handler) pourFeed(ctx context.Context, req *muxrpc.Request) error {
	// check & parse args
	if len(req.Args) < 1 {
		return errors.New("not enough arguments, expecting feed id")
	}
	qv := req.Args[0].(map[string]interface{})
	if id, ok := qv["id"]; ok && id == `@S9lG7keeOfIAepU1LRD9XUDtuesbejRC2+3/FRaGZ3s=.sha256` {
		h.Info.Log("pour", "ignored sha ID")
		return nil // no comment
	}
	var qry message.CreateHistArgs
	if err := mapstructure.Decode(qv, &qry); err != nil {
		return errors.Wrap(err, "failed#2 to decode qry map")
	}
	if qry.Id == "@S9lG7keeOfIAepU1LRD9XUDtuesbejRC2+3/FRaGZ3s=.sha256" { // nothing to do here...
		return nil
	}
	feedRef, err := ssb.ParseFeedRef(qry.Id)
	if err != nil {
		return errors.Wrapf(err, "invalid ref: %q", qry.Id)
	}

	// check what we got
	userLog, err := h.UserFeeds.Get(librarian.Addr(feedRef.ID))
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
	case margaret.BaseSeq:
		if qry.Seq >= int64(v) { // more than we got
			return errors.Wrap(req.Stream.Close(), "pour: failed to close")
		}
		if qry.Seq >= 1 {
			qry.Seq-- // out internals are 0-indexed
		}

		if qry.Live && qry.Limit == 0 {
			// currently having live streams is not implemented
			// it might work but we have some problems with dangling rpc routines which we like to fix first
			qry.Limit = -1
		}

		userSequences, err := userLog.Query(margaret.Gte(margaret.BaseSeq(qry.Seq)), margaret.Limit(int(qry.Limit)))
		if err != nil {
			return errors.Wrapf(err, "invalid user log query seq:%d - limit:%d", qry.Seq, qry.Limit)
		}

		sent := 0
		wrapSink := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
			v, err := h.RootLog.Get(val.(margaret.Seq))
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

		if h.sysCtr != nil {
			h.sysCtr.With("event", "gossiptx").Add(float64(sent))
		}
	default:
		return errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", latest)
	}
	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}
