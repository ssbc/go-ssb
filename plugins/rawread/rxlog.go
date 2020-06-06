// SPDX-License-Identifier: MIT

package rawread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
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
	fmt.Fprintln(os.Stderr, "createLogStream args:", string(req.RawArgs))
	var qry message.CreateLogArgs
	if len(req.Args()) == 1 {
		var args []message.CreateLogArgs
		err := json.Unmarshal(req.RawArgs, &args)
		if err != nil {
			fmt.Fprintln(os.Stderr, "createLogStream err:", err)
			req.CloseWithError(errors.Wrap(err, "bad request data"))
			return
		}
		if len(args) == 1 {
			qry = args[0]
			qry.Values = true
			qry.Keys = true
			qry.Limit = 1312
			spew.Dump(qry)
		}
	} else {
		qry.Keys = true
		qry.Limit = -1
	}

	// // only return message keys
	// qry.Values = true

	if qry.Gt == -1 {
		// sv, err := g.root.Seq().Value()
		// if err != nil {
		// 	fmt.Fprintln(os.Stderr, "createLogStream err:", err)
		// 	req.CloseWithError(errors.Wrap(err, "logStream: failed to qry current seq"))
		// 	return
		// }

		// seq := sv.(margaret.Seq)
		// qry.Seq = seq.Seq() - 1
		qry.Keys = true
		qry.Seq = 0
	}

	start := time.Now()
	src, err := g.root.Query(
		margaret.SeqWrap(false),
		//		margaret.Gte(margaret.BaseSeq(qry.Seq)),
		margaret.Limit(int(qry.Limit)),
		margaret.Live(qry.Live),
		margaret.Reverse(qry.Reverse),
	)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "logStream: failed to qry tipe"))
		return
	}

	err = luigi.Pump(ctx, transform.NewKeyValueWrapper(req.Stream, qry.Keys), src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "createLogStream err:", err)
		req.CloseWithError(errors.Wrap(err, "logStream: failed to pump msgs"))
		return
	}
	req.Stream.Close()
	fmt.Fprintln(os.Stderr, "createLogStream closed:", err, "after:", time.Since(start))
}
