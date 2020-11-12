// SPDX-License-Identifier: MIT

package rawread

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/muxmux"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

type Plugin struct {
	root  margaret.Log
	types *roaring.MultiLog
	res   *repo.SequenceResolver

	h muxrpc.Handler
}

func NewByTypePlugin(log log.Logger, rootLog margaret.Log, ml *roaring.MultiLog, res *repo.SequenceResolver) ssb.Plugin {
	plug := &Plugin{
		root:  rootLog,
		types: ml,
		res:   res,
	}

	h := muxmux.New(log)
	h.RegisterSource(muxrpc.Method{"messagesByType"}, plug)

	plug.h = &h
	return plug
}

func (lt Plugin) Name() string { return "msgTypes" }

func (Plugin) Method() muxrpc.Method {
	return muxrpc.Method{"messagesByType"}
}
func (lt Plugin) Handler() muxrpc.Handler {
	return lt.h
}

func (g Plugin) HandleSource(ctx context.Context, req *muxrpc.Request, snk luigi.Sink) error {
	args := req.Args()
	if len(args) < 1 {
		return errors.Errorf("invalid arguments")
	}
	var qry message.MessagesByTypeArgs

	switch v := args[0].(type) {

	case map[string]interface{}:
		q, err := message.NewCreateHistArgsFromMap(v)
		if err != nil {
			return errors.Wrap(err, "bad request")
		}
		qry.CommonArgs = q.CommonArgs
		qry.StreamArgs = q.StreamArgs

		var ok bool
		qry.Type, ok = v["type"].(string)
		if !ok {
			return errors.Errorf("bad request - missing root")
		}

	case string:
		qry.Limit = -1
		qry.Type = v
		qry.Keys = true

	default:
		return errors.Errorf("invalid argument type %T", args[0])
	}

	start := time.Now()

	snk = transform.NewKeyValueWrapper(req.Stream, qry.Keys)
	var cnt int
	snk = newSinkCounter(&cnt, snk)

	if qry.Live {
		typed, err := g.types.Get(librarian.Addr(qry.Type))
		if err != nil {
			return errors.Wrap(err, "failed to load typed log")
		}

		src, err := mutil.Indirect(g.root, typed).Query(margaret.Limit(int(qry.Limit)), margaret.Live(qry.Live), margaret.Reverse(qry.Reverse))
		if err != nil {
			return errors.Wrap(err, "logT: failed to qry tipe")
		}

		err = luigi.Pump(ctx, snk, src)
		if err != nil {
			return errors.Wrap(err, "logT: failed to pump msgs")
		}

		fmt.Println("bytype", qry.Type, cnt)
		return snk.Close()
	}

	// not live
	typed, err := g.types.LoadInternalBitmap(librarian.Addr(qry.Type))
	if err != nil {
		return errors.Wrap(err, "failed to load typed log")
	}

	// TODO: set _all_ correctly if gt=0 && lt=0
	if qry.Lt == 0 {
		qry.Lt = math.MaxInt64
	}

	var filter = func(ts int64) bool {
		isGreater := ts > qry.Gt
		isSmaller := ts < qry.Lt
		return isGreater && isSmaller
	}

	sort, err := g.res.SortAndFilterBitmap(typed, repo.SortByClaimed, filter, qry.Reverse)
	if err != nil {
		return errors.Wrap(err, "failed to filter bitmap")
	}

	for _, res := range sort {
		v, err := g.root.Get(margaret.BaseSeq(res.Seq))
		if err != nil {
			fmt.Fprintln(os.Stderr, "messagesByType failed to get seq:", res.Seq, " with:", err, "after:", time.Since(start))
			continue
		}

		// TODO: skip nulled

		if err := snk.Pour(ctx, v); err != nil {
			fmt.Fprintln(os.Stderr, "messagesByType failed send:", res.Seq, " with:", err, "after:", time.Since(start))
			break
		}

		if qry.Limit >= 0 {
			qry.Limit--
			if qry.Limit == 0 {
				break
			}
		}
	}
	fmt.Println("bytype", qry.Type, cnt)
	return snk.Close()
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
