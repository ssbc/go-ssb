// SPDX-License-Identifier: MIT

package rawread

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
)

// ~> sbot createSequenceStream --help
// (log) Fetch messages ordered by the time received.
// log [--live] [--gt index] [--gte index] [--lt index] [--lte index] [--reverse]  [--keys] [--values] [--limit n]
type seqStream struct {
	h muxrpc.Handler
}

func NewSequenceStream(rootLog margaret.Log) ssb.Plugin {
	plug := &seqStream{}
	plug.h = seqStreamHandler{
		root: rootLog,
	}
	return plug
}

func (lt seqStream) Name() string { return "createSequenceStream" }

func (seqStream) Method() muxrpc.Method {
	return muxrpc.Method{"createSequenceStream"}
}
func (lt seqStream) Handler() muxrpc.Handler {
	return lt.h
}

type seqStreamHandler struct {
	root margaret.Log
}

func (g seqStreamHandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
}

func (g seqStreamHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	fmt.Fprintln(os.Stderr, "seqStream args:", string(req.RawArgs))
	// if len(req.Args()) != 1 {
	// 	goon.Dump(req.RawArgs)
	// 	req.CloseWithError(errors.Errorf("invalid arguments"))
	// 	return
	// }

	// var args []message.CreateLogArgs
	// err := json.Unmarshal(req.RawArgs, &args)
	// if err != nil {
	// 	req.CloseWithError(errors.Wrap(err, "bad request data"))
	// 	return
	// }
	// if len(args) != 1 {
	// 	req.CloseWithError(errors.Wrap(err, "bad request"))
	// 	return
	// }

	// qry := args[0]

	// if qry.Live {
	// 	qry.Limit = -1
	// }

	sv, err := g.root.Seq().Value()
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "seqStream: failed to qry current seq"))
		return
	}

	seq := sv.(margaret.Seq)

	err = req.Stream.Pour(ctx, seq.Seq())
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "seqStream: failed to send current sequence"))
		return
	}

	newSeq := make(chan int64)

	cancel := g.root.Seq().Register(luigi.FuncSink(func(ctx context.Context, iv interface{}, err error) error {
		if err != nil {
			return err
		}
		sw, ok := iv.(margaret.SeqWrapper)
		if !ok {
			return errors.Errorf("not a seq wrapper")
		}
		newSeq <- sw.Seq().Seq()
		return nil
	}))

	go func() {
		last := time.Now()
		for {
			var seq int64
			var ok bool
			select {
			case sv, ok = <-newSeq:
				if !ok {
					cancel()
					return
				}

			case <-ctx.Done():
				cancel()
			}
			if time.Since(last) > 1*time.Second {
				err := req.Stream.Pour(ctx, seq)
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

}
