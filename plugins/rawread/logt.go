package rawread

import (
	"context"

	"go.cryptoscope.co/ssb/message"

	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/ssb/internal/mutil"

	"go.cryptoscope.co/librarian"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
)

//  messagesByType --help
// (logt) Retrieve messages with a given type, ordered by receive-time.
// logt --type {type} [--gt index] [--gte index] [--lt index] [--lte index] [--reverse]  [--keys] [--values] [--limit n]
type logTplug struct {
	h muxrpc.Handler
}

func NewByType(rootLog margaret.Log, typeLogs multilog.MultiLog) ssb.Plugin {
	plug := &logTplug{}
	plug.h = logThandler{
		root:  rootLog,
		types: typeLogs,
	}
	return plug
}

func (lt logTplug) Name() string { return "messagesByType" }

func (logTplug) Method() muxrpc.Method {
	return muxrpc.Method{"messagesByType"}
}
func (lt logTplug) Handler() muxrpc.Handler {
	return lt.h
}

type logThandler struct {
	root  margaret.Log
	types multilog.MultiLog
}

func (g logThandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
}

func (g logThandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
		return
	}
	var (
		tipe librarian.Addr
		qry  message.CreateHistArgs
	)
	switch v := req.Args[0].(type) {
	case string:
		tipe = librarian.Addr(v)
	case map[string]interface{}:
		mv, ok := req.Args[0].(map[string]interface{})
		if !ok {
			req.CloseWithError(errors.Errorf("bad request"))
			return
		}
		q, err := message.NewCreateHistArgsFromMap(mv)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "bad request"))
			return
		}
		qry = *q
	default:
		req.CloseWithError(errors.Errorf("invalid argument type %T", req.Args[0]))
		return
	}

	if len(req.Args) == 2 {
		spew.Dump(req.Args)
		mv, ok := req.Args[1].(map[string]interface{})
		if !ok {
			req.CloseWithError(errors.Errorf("bad request"))
			return
		}
		q, err := message.NewCreateHistArgsFromMap(mv)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "bad request"))
			return
		}
		qry = *q
	} else {
		qry.Limit = -1
		// TODO: msg should be wrapped in obj with key and rxt
		qry.Keys = true

		// only return message keys
		qry.Values = true
	}

	tipeLog, err := g.types.Get(tipe)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logT: no multilog for tipe?"))
		return
	}

	resolved := mutil.Indirect(g.root, tipeLog)
	src, err := resolved.Query(margaret.Limit(int(qry.Limit)), margaret.Live(qry.Live))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logT: failed to qry tipe"))
		return
	}

	snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			return err
		}
		msg := v.(message.StoredMessage)
		return req.Stream.Pour(ctx, message.RawSignedMessage{RawMessage: msg.Raw})
	})

	err = luigi.Pump(ctx, snk, src)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logT: failed to pump msgs"))
		return
	}

	req.Stream.Close()
}
