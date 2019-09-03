package gossip

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/codec"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/message/multimsg"
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

	feedRef, err := ssb.ParseFeedRef(qry.ID)
	if err != nil {
		return nil // only handle valid feed refs
	}

	// check what we got
	userLog, err := h.UserFeeds.Get(feedRef.StoredAddr())
	if err != nil {
		return errors.Wrapf(err, "failed to open sublog for user")
	}
	latest, err := userLog.Seq().Value()
	if err != nil {
		return errors.Wrapf(err, "failed to observe latest")
	}

	var (
		src luigi.Source
		snk luigi.Sink
	)

	// act accordingly
	switch v := latest.(type) {
	case librarian.UnsetValue: // don't have the feed - nothing to do?
		return req.Stream.Close()
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
		src, err = resolved.Query(
			margaret.Gte(margaret.BaseSeq(qry.Seq)),
			margaret.Limit(int(qry.Limit)),
			margaret.Live(false),
			margaret.Reverse(qry.Reverse),
		)
		if err != nil {
			return errors.Wrapf(err, "invalid user log query seq:%d - limit:%d", qry.Seq, qry.Limit)
		}

	default:
		return errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", latest)
	}

	// sent := 0 TODO wrap snk and count there

	switch feedRef.Algo {
	case ssb.RefAlgoFeedSSB1:
		src = transform.NewKeyValueWrapper(src, qry.Keys)
		snk = legacyStreamSink(req.Stream)
	case ssb.RefAlgoFeedGabby:
		switch {
		case qry.AsJSON && !qry.Keys:
			snk = asJSONsink(req.Stream)

		case qry.AsJSON && qry.Keys:
			src = transform.NewKeyValueWrapper(src, true)
			snk = legacyStreamSink(req.Stream)
		default:
			snk = gabbyStreamSink(req.Stream)
		}
	default:
		return errors.Errorf("unsupported feed format.")
	}

	err = luigi.Pump(ctx, snk, src)
	// if h.sysCtr != nil {
	// 	h.sysCtr.With("event", "gossiptx").Add(float64(sent))
	// } else {
	// 	h.Info.Log("event", "gossiptx", "n", sent)
	// }

	if errors.Cause(err) == context.Canceled {
		// req.Stream.Close()
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to pump messages to peer")
	}

	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}

func legacyStreamSink(stream luigi.Sink) luigi.Sink {
	return luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			return err
		}
		msg, ok := v.(*transform.KeyValue)
		if !ok {
			return errors.Errorf("legacySink: expected %T - got %T", msg, v)
		}
		return stream.Pour(ctx, json.RawMessage(msg.Data))
	})
}

func gabbyStreamSink(stream luigi.Sink) luigi.Sink {
	return luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			return err
		}
		mm, ok := v.(*multimsg.MultiMessage)
		if !ok {
			return errors.Errorf("binStream: expected []byte - got %T", v)
		}
		tr, ok := mm.AsGabby()
		if !ok {
			return errors.Errorf("wrong mm type")
		}

		trdata, err := tr.MarshalCBOR()
		if err != nil {
			return errors.Wrap(err, "failed to marshal transfer")
		}

		return stream.Pour(ctx, codec.Body(trdata))
	})
}

func asJSONsink(stream luigi.Sink) luigi.Sink {
	return luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return stream.Close()
			}
			return err
		}
		msg, ok := val.(ssb.Message)
		if !ok {
			return errors.Errorf("asJSONsink: expected ssb.Message - got %T", val)
		}
		return stream.Pour(ctx, msg.ValueContentJSON())
	})
}
