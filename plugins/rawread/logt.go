// SPDX-License-Identifier: MIT

package rawread

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"

	bmap "github.com/RoaringBitmap/roaring"
	"github.com/davecgh/go-spew/spew"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc/v2"
	"go.mindeco.de/log"

	"go.cryptoscope.co/muxrpc/v2/typemux"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
)

type Plugin struct {
	rxlog margaret.Log
	types *roaring.MultiLog

	priv    *roaring.MultiLog
	isSelf  ssb.Authorizer
	unboxer *private.Manager

	res *repo.SequenceResolver

	h muxrpc.Handler
}

func NewByTypePlugin(
	log log.Logger,
	rootLog margaret.Log,
	ml *roaring.MultiLog,
	pl *roaring.MultiLog,
	pm *private.Manager,
	// TODO[major/pgroups] fix storage and resumption
	// res *repo.SequenceResolver,
	isSelf ssb.Authorizer,
) ssb.Plugin {
	plug := &Plugin{
		rxlog: rootLog,
		types: ml,

		priv: pl,

		unboxer: pm,

		// TODO[major/pgroups] fix storage and resumption
		// res: res,

		isSelf: isSelf,
	}

	h := typemux.New(log)
	h.RegisterSource(muxrpc.Method{"messagesByType"}, plug)

	plug.h = &h
	return plug
}

func (lt Plugin) Name() string            { return "msgTypes" }
func (Plugin) Method() muxrpc.Method      { return muxrpc.Method{"messagesByType"} }
func (lt Plugin) Handler() muxrpc.Handler { return lt.h }

func (g Plugin) HandleSource(ctx context.Context, req *muxrpc.Request, w *muxrpc.ByteSink) error {
	var qry message.MessagesByTypeArgs
	var args []message.MessagesByTypeArgs

	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {

		// assume just string for type
		var args []string
		err := json.Unmarshal(req.RawArgs, &args)
		if err != nil {
			return fmt.Errorf("bad request data: %w", err)
		}
		if len(args) != 1 {
			return fmt.Errorf("bad request data: assumed string argument for type field")
		}
		qry.Type = args[0]
		// Defaults for no arguments
		qry.Keys = true
		qry.Limit = -1

	} else {
		nargs := len(args)
		if nargs == 1 {
			qry = args[0]
		} else {
			return fmt.Errorf("bad request data: assumed one argument object but got %d", nargs)
		}
	}

	remote, err := ssb.GetFeedRefFromAddr(req.RemoteAddr())
	if err != nil {
		return fmt.Errorf("failed to establish remote: %w", err)
	}

	isSelf := g.isSelf.Authorize(remote)
	if qry.Private && isSelf != nil {
		return fmt.Errorf("not authroized")
	}

	// create toJSON sink
	snk := transform.NewKeyValueWrapper(w, qry.Keys)

	// wrap it into a counter for debugging
	var cnt int
	snk = newSinkCounter(&cnt, snk)

	idxAddr := librarian.Addr("string:" + qry.Type)
	if true { // TODO[major/pgroups] fix storage and resumption
		if qry.Private {
			return fmt.Errorf("TODO: fix live && private")
		}
		typed, err := g.types.Get(idxAddr)
		if err != nil {
			return fmt.Errorf("failed to load typed log: %w", err)
		}

		src, err := mutil.Indirect(g.rxlog, typed).Query(
			margaret.Limit(int(qry.Limit)),
			margaret.Live(qry.Live))
		if err != nil {
			return fmt.Errorf("logT: failed to qry tipe: %w", err)
		}

		// if qry.Private { TODO
		// 	src = g.unboxedSrc(src)
		// }

		err = luigi.Pump(ctx, snk, src)
		if err != nil {
			return fmt.Errorf("logT: failed to pump msgs: %w", err)
		}

		return snk.Close()
	}
	// TODO[major/pgroups] fix storage and resumption
	// TODO: fix seq resolver filling hiccups
	return snk.Close()

	/* TODO: i'm skipping a fairly big refactor here to find out what works first.
	   ideallly the live and not-live code would just be the same, somehow shoving it into Query(...).
	   Same goes for timestamp sorting and private.
	   Private is at least orthogonal, whereas sorting and live don't go well together.
	*/

	// not live
	typed, err := g.types.LoadInternalBitmap(idxAddr)
	if err != nil {
		return snk.Close()
		//		return errors.Wrap(err, "failed to load typed log")
	}

	if qry.Private {
		snk = g.unboxer.WrappedUnboxingSink(snk)
	} else {
		// filter all boxed messages from the stream
		box1, err := g.priv.LoadInternalBitmap(librarian.Addr("meta:box1"))
		if err != nil {
			// TODO: compare not found
			// return errors.Wrap(err, "failed to load bmap for box1")
			box1 = bmap.New()
		}

		box2, err := g.priv.LoadInternalBitmap(librarian.Addr("meta:box2"))
		if err != nil {
			// TODO: compare not found
			// return errors.Wrap(err, "failed to load bmap for box2")
			box2 = bmap.New()
		}

		box1.Or(box2) // all the boxed messages

		// remove all the boxed ones from the type we are looking up
		typed.AndNot(box1)
	}

	// TODO: set _all_ correctly if gt=0 && lt=0
	if qry.Lt == 0 {
		qry.Lt = math.MaxInt64
	}

	spew.Dump(qry)

	var filter = func(ts int64) bool {
		isGreater := ts > qry.Gt
		isSmaller := ts < qry.Lt
		return isGreater && isSmaller
	}

	sort, err := g.res.SortAndFilterBitmap(typed, repo.SortByClaimed, filter, qry.Reverse)
	if err != nil {
		return fmt.Errorf("failed to filter bitmap: %w", err)
	}

	for _, res := range sort {
		v, err := g.rxlog.Get(margaret.BaseSeq(res.Seq))
		if err != nil {
			if margaret.IsErrNulled(err) {
				continue
			}
			fmt.Fprintln(os.Stderr, "messagesByType failed to get seq:", res.Seq, " with:", err)
			continue
		}

		if err := snk.Pour(ctx, v); err != nil {
			fmt.Fprintln(os.Stderr, "messagesByType failed send:", res.Seq, " with:", err)
			break
		}

		if qry.Limit >= 0 {
			qry.Limit--
			if qry.Limit == 0 {
				break
			}
		}
	}
	fmt.Println("streamed", cnt, " for type:", qry.Type)
	return snk.Close()
}

func newSinkCounter(counter *int, sink luigi.Sink) luigi.FuncSink {
	return func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			return err
		}

		*counter++
		return sink.Pour(ctx, v)
	}
}
