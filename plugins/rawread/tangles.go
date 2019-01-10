package rawread

import (
	"context"

	"go.cryptoscope.co/ssb/internal/mutil"

	"go.cryptoscope.co/librarian"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
)

type tanglePlug struct {
	h muxrpc.Handler
}

func NewTanglePlug(root margaret.Log, tangle multilog.MultiLog) ssb.Plugin {
	plug := &tanglePlug{}
	plug.h = tangleHandler{
		root:   root,
		tangle: tangle,
	}
	return plug
}

func (lt tanglePlug) Name() string { return "tangles" }

func (tanglePlug) Method() muxrpc.Method {
	return muxrpc.Method{"tangles"}
}
func (lt tanglePlug) Handler() muxrpc.Handler {
	return lt.h
}

type tangleHandler struct {
	root   margaret.Log
	tangle multilog.MultiLog
}

func (g tangleHandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (g tangleHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
		return
	}
	var qry struct {
		message.CreateHistArgs
		Root *ssb.MessageRef
	}

	switch v := req.Args[0].(type) {

	case map[string]interface{}:
		q, err := message.NewCreateHistArgsFromMap(v)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "bad request"))
			return
		}
		qry.CreateHistArgs = *q

		root, ok := v["root"].(string)
		if !ok {
			req.CloseWithError(errors.Errorf("bad request - missing root"))
			return
		}

		qry.Root, err = ssb.ParseMessageRef(root)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "bad request - invalid root"))
			return
		}
	default:
		req.CloseWithError(errors.Errorf("invalid argument type %T", req.Args[0]))
		return
	}

	if qry.Live {
		qry.Limit = -1
	}

	threadLog, err := g.tangle.Get(librarian.Addr(qry.Root.Hash))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to load thread"))
		return
	}

	src, err := mutil.Indirect(g.root, threadLog).Query(margaret.Limit(int(qry.Limit)), margaret.Live(qry.Live), margaret.Reverse(qry.Reverse))
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
