// SPDX-License-Identifier: MIT

package bytype

import (
	"context"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins2"
)

type Plugin struct {
	h handler
}

var (
	_ plugins2.NeedsRootLog = (*Plugin)(nil)
)

// TODO: return plugin spec similar to margaret qry spec?

func (tp *Plugin) WantRootLog(rl margaret.Log) error {
	tp.h.root = rl
	return nil
}

func (lt Plugin) Name() string            { return "msgTypes" }
func (Plugin) Method() muxrpc.Method      { return muxrpc.Method{"messagesByType"} }
func (lt Plugin) Handler() muxrpc.Handler { return lt.h }

type handler struct {
	root  margaret.Log
	types multilog.MultiLog
}

func (g handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (g handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	args := req.Args()
	if len(args) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
		return
	}
	var qry message.MessagesByTypeArgs

	switch v := args[0].(type) {

	case map[string]interface{}:
		q, err := message.NewCreateHistArgsFromMap(v)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "bad request"))
			return
		}
		qry.CommonArgs = q.CommonArgs
		qry.StreamArgs = q.StreamArgs

		var ok bool
		qry.Type, ok = v["type"].(string)
		if !ok {
			req.CloseWithError(errors.Errorf("bad request - missing root"))
			return
		}

	case string:
		qry.Limit = -1
		qry.Type = v
		qry.Keys = true

	default:
		req.CloseWithError(errors.Errorf("invalid argument type %T", args[0]))
		return
	}
	spew.Dump(qry)
	typed, err := g.types.Get(librarian.Addr(qry.Type))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to load typed log"))
		return
	}

	src, err := mutil.Indirect(g.root, typed).Query(margaret.Limit(int(qry.Limit)), margaret.Live(qry.Live), margaret.Reverse(qry.Reverse))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logT: failed to qry tipe"))
		return
	}

	snk := transform.NewKeyValueWrapper(req.Stream, qry.Keys)

	var cnt int

	err = luigi.Pump(ctx, newSinkCounter(&cnt, snk), src)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logT: failed to pump msgs"))
		return
	}

	fmt.Println("bytype", qry.Type, cnt)

	req.Stream.Close()
}

func newSinkCounter(counter *int, sink luigi.Sink) luigi.FuncSink {
	return func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			fmt.Println("weird", err)
			return err
		}

		*counter++
		return sink.Pour(ctx, v)
	}
}
