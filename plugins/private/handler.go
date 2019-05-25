package private

import (
	"context"
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/private"

	"go.cryptoscope.co/ssb"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
)

type handler struct {
	info logging.Interface

	publish margaret.Log
	read    margaret.Log
}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	var closed bool
	checkAndClose := func(err error) {
		if err != nil {
			h.info.Log("event", "closing", "method", req.Method, "err", err)
		}
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			err := errors.Wrapf(closeErr, "error closeing request")
			if err != nil {
				h.info.Log("event", "closing", "method", req.Method, "err", err)
			}
		}
	}

	defer func() {
		if !closed {
			err := errors.Wrapf(req.Stream.Close(), "gossip: error closing call")
			if err != nil {
				h.info.Log("event", "closing", "method", req.Method, "err", err)
			}
		}
	}()

	switch req.Method.String() {

	case "private.publish":
		if req.Type == "" {
			req.Type = "async"
		}
		if n := len(req.Args); n != 2 {
			req.CloseWithError(errors.Errorf("private/publish: bad request. expected 2 argument got %d", n))
			return
		}

		msg, err := json.Marshal(req.Args[0])
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "failed to encode message"))
			return
		}

		rcps, ok := req.Args[1].([]interface{})
		if !ok {
			req.CloseWithError(errors.Errorf("private/publish: wrong argument type. expected []strings but got %T", req.Args[1]))
			return
		}

		rcpsRefs := make([]*ssb.FeedRef, len(rcps))
		for i, rv := range rcps {
			rstr, ok := rv.(string)
			if !ok {
				req.CloseWithError(errors.Errorf("private/publish: wrong argument type. expected strings but got %T", rv))
				return
			}
			rcpsRefs[i], err = ssb.ParseFeedRef(rstr)
			if err != nil {
				req.CloseWithError(errors.Wrapf(err, "private/publish: failed to parse recp %d", i))
				return
			}
		}

		boxedMsg, err := private.Box(msg, rcpsRefs...)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "private/publish: failed to box message"))
			return
		}

		seq, err := h.publish.Append(boxedMsg)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "private/publish: pour failed"))
			return
		}

		h.info.Log("published", seq.Seq())

		err = req.Return(ctx, fmt.Sprintf("published msg: %d", seq.Seq()))
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "private/publish: return failed"))
			return
		}

		return

	case "private.read":
		if req.Type != "source" {
			checkAndClose(errors.Errorf("private.read: wrong request type. %s", req.Type))
			return
		}
		var qry message.CreateHistArgs

		switch v := req.Args[0].(type) {

		case map[string]interface{}:
			q, err := message.NewCreateHistArgsFromMap(v)
			if err != nil {
				req.CloseWithError(errors.Wrap(err, "bad request"))
				return
			}
			qry = *q
		default:
			req.CloseWithError(errors.Errorf("invalid argument type %T", req.Args[0]))
			return
		}

		if qry.Live {
			qry.Limit = -1
		}

		src, err := h.read.Query(
			margaret.Gte(margaret.BaseSeq(qry.Seq)),
			margaret.Limit(int(qry.Limit)),
			margaret.Live(qry.Live))
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "private/publish: return failed"))
			return
		}

		snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			if err != nil {
				return err
			}
			msg, ok := v.([]byte)
			if !ok {
				return errors.Errorf("private pour: expected %T - got %T", msg, v)
			}
			return req.Stream.Pour(ctx, json.RawMessage(msg))
		})

		err = luigi.Pump(ctx, snk, transform.NewKeyValueWrapper(src, qry.Keys))
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "private/publish: return failed"))
			return
		}
		req.Close()

	default:
		checkAndClose(errors.Errorf("private: unknown command: %s", req.Method))
	}

}

func (h handler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}
