package gossip

import (
	"context"

	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/transform"

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
		return errors.New("ssb/message: not enough arguments, expecting feed id")
	}
	argMap, ok := req.Args[0].(map[string]interface{})
	if !ok {
		return errors.Errorf("ssb/message: not the right map - %T", req.Args[0])
	}
	qry, err := message.NewCreateHistArgsFromMap(argMap)
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
		if qry.Seq != 0 {
			qry.Seq--               // our idx is 0 based
			if qry.Seq > int64(v) { // more than we got
				return errors.Wrap(req.Stream.Close(), "pour: failed to close")
			}
		}

		if qry.Limit == 0 {
			// currently having live streams is not tested
			// it might work but we have some problems with dangling rpc routines which we like to fix first
			qry.Limit = -1
		}

		resolved := mutil.Indirect(h.RootLog, userLog)
		src, err := resolved.Query(
			margaret.Gte(margaret.BaseSeq(qry.Seq)),
			margaret.Limit(int(qry.Limit)),
			margaret.Live(qry.Live),
			margaret.Reverse(qry.Reverse),
		)
		if err != nil {
			return errors.Wrapf(err, "invalid user log query seq:%d - limit:%d", qry.Seq, qry.Limit)
		}

		sent := 0
		snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			if err != nil {
				return err
			}
			msg, ok := v.([]byte)
			if !ok {
				return errors.Errorf("b4pour: expected []byte - got %T", v)
			}
			sent++
			return req.Stream.Pour(ctx, message.RawSignedMessage{RawMessage: msg})
		})

		err = luigi.Pump(ctx, snk, transform.NewKeyValueWrapper(src, qry.Keys))
		if h.sysCtr != nil {
			h.sysCtr.With("event", "gossiptx").Add(float64(sent))
		} else {
			h.Info.Log("event", "gossiptx", "n", sent)
		}
		if errors.Cause(err) == context.Canceled {
			req.Stream.Close()
			return nil
		} else if err != nil {
			return errors.Wrap(err, "failed to pump messages to peer")
		}

	default:
		return errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", latest)
	}
	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}
