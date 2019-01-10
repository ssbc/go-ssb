package rawread

import (
	"context"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
)

// ~> sbot createLogStream --help
// (log) Fetch messages ordered by the time received.
// log [--live] [--gt index] [--gte index] [--lt index] [--lte index] [--reverse]  [--keys] [--values] [--limit n]
type rxLogPlug struct {
	h muxrpc.Handler
}

func NewRXLog(rootLog margaret.Log) ssb.Plugin {
	plug := &rxLogPlug{}
	plug.h = rxLogHandler{
		root: rootLog,
	}
	return plug
}

func (lt rxLogPlug) Name() string { return "createLogStream" }

func (rxLogPlug) Method() muxrpc.Method {
	return muxrpc.Method{"createLogStream"}
}
func (lt rxLogPlug) Handler() muxrpc.Handler {
	return lt.h
}

type rxLogHandler struct {
	root margaret.Log
}

func (g rxLogHandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
}

func (g rxLogHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
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

	// // only return message keys
	// qry.Values = true

	src, err := g.root.Query(margaret.Limit(int(qry.Limit)), margaret.Live(qry.Live), margaret.Reverse(qry.Reverse))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logT: failed to qry tipe"))
		return
	}

	snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			return err
		}
		msg, ok := v.([]byte)
		if !ok {
			return errors.Errorf("b4pour: expected []byte - got %T", v)
		}
		return req.Stream.Pour(ctx, message.RawSignedMessage{RawMessage: msg})
	})

	err = luigi.Pump(ctx, snk, transform.NewKeyValueWrapper(src, qry.Keys))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logT: failed to pump msgs"))
		return
	}

	req.Stream.Close()
}
