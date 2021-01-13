// SPDX-License-Identifier: MIT

package rawread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"

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
	// fmt.Fprintln(os.Stderr, "createLogStream args:", string(req.RawArgs))
	var qry message.CreateLogArgs
	var args []message.CreateLogArgs
	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "createLogStream err:", err)
		req.CloseWithError(fmt.Errorf("bad request data: %w", err))
		return
	}
	if len(args) == 1 {
		qry = args[0]
	} else {
		// Defaults for no arguments
		qry.Keys = true
		qry.Limit = -1
	}

	// empty query doesn't make much sense...
	if qry.Limit == 0 {
		qry.Limit = -1
	}

	// only return message keys
	// qry.Values = true

	if qry.Gt == -1 {
		sv, err := g.root.Seq().Value()
		if err != nil {
			fmt.Fprintln(os.Stderr, "createLogStream err:", err)
			req.CloseWithError(fmt.Errorf("logStream: failed to qry current seq: %w", err))
			return
		}

		seq := sv.(margaret.Seq)
		qry.Seq = seq.Seq() - 1
	}

	// start := time.Now()
	src, err := g.root.Query(
		margaret.SeqWrap(false),
		margaret.Gte(margaret.BaseSeq(qry.Seq)),
		margaret.Limit(int(qry.Limit)),
		margaret.Live(qry.Live),
		margaret.Reverse(qry.Reverse),
	)
	if err != nil {
		req.CloseWithError(fmt.Errorf("logStream: failed to qry tipe: %w", err))
		return
	}

	snk, err := req.GetResponseSink()
	if err != nil {
		req.CloseWithError(err)
		return
	}
	err = luigi.Pump(ctx, transform.NewKeyValueWrapper(snk, qry.Keys), src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "createLogStream err:", err)
		req.CloseWithError(fmt.Errorf("logStream: failed to pump msgs: %w", err))
		return
	}
	snk.Close()
	// fmt.Fprintln(os.Stderr, "createLogStream closed:", err, "after:", time.Since(start))
}
