package gossip

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

func newCreateHistArgs(args []interface{}) (*message.CreateHistArgs, error) {
	// check & parse args
	if len(args) < 1 {
		return nil, errors.New("ssb/message: not enough arguments, expecting feed id")
	}
	argMap, ok := args[0].(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("ssb/message: not the right map - %T", args[0])
	}

	// could reflect over qrys fiields but meh - compiler knows better
	var qry message.CreateHistArgs
	for k, v := range argMap {
		switch k = strings.ToLower(k); k {
		case "live", "keys", "values", "reverse":
			b, ok := v.(bool)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a bool for %s", k)
			}
			switch k {
			case "live":
				qry.Live = b
			case "keys":
				qry.Keys = b
			case "values":
				qry.Values = b
			case "reverse":
				qry.Reverse = b
			}
		case "id":
			qry.Id, ok = v.(string)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a string for %s", k)
			}
		case "seq", "limit":
			n, ok := v.(float64)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a float64 for %s", k)
			}
			switch k {
			case "seq":
				qry.Seq = int64(n)
			case "limit":
				qry.Limit = int64(n)
			}
		}
	}

	return &qry, nil
}

func (h *handler) pourFeed(ctx context.Context, req *muxrpc.Request) error {

	qry, err := newCreateHistArgs(req.Args)
	if err != nil {
		return errors.Wrap(err, "bad request")
	}

	feedRef, err := ssb.ParseFeedRef(qry.Id)
	if err != nil {
		return nil // only handle valid feed refs
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

		err = luigi.Pump(ctx, wrapSink, userSequences)
		if h.sysCtr != nil {
			h.sysCtr.With("event", "gossiptx").Add(float64(sent))
		} else {
			h.Info.Log("event", "gossiptx", "n", sent)
		}
		if err != nil {
			return errors.Wrap(err, "failed to pump messages to peer")
		}

	default:
		return errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", latest)
	}
	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}
