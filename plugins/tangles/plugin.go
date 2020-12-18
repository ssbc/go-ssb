// SPDX-License-Identifier: MIT

package tangles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	bmap "github.com/RoaringBitmap/roaring"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/muxrpc/v2/typemux"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/private"
	refs "go.mindeco.de/ssb-refs"
)

type Plugin struct {
	h muxrpc.Handler
}

func NewPlugin(rxlog margaret.Log, tangles, private *roaring.MultiLog, unboxer *private.Manager, isSelf ssb.Authorizer) *Plugin {
	mux := typemux.New(log.NewNopLogger())

	mux.RegisterSource(muxrpc.Method{"tangles", "replies"}, tangleHandler{
		rxlog:   rxlog,
		tangles: tangles,

		// private utils
		private: private,
		unboxer: unboxer,
		isSelf:  isSelf,
	})

	/* TODO: heads
			mux.RegisterAsync(muxrpc.Method{"tangles", "heads"}, headsHandler{
			rxlog:   rxlog,
			tangles: threads,

	  		// private utils
			private: private,
			unboxer: unboxer,
			isSelf:  isSelf,
		})
	*/

	return &Plugin{
		h: &mux,
	}
}

func (lt Plugin) Name() string            { return "tangles" }
func (Plugin) Method() muxrpc.Method      { return muxrpc.Method{"tangles"} }
func (lt Plugin) Handler() muxrpc.Handler { return lt.h }

type tangleHandler struct {
	rxlog   margaret.Log
	tangles *roaring.MultiLog
	private *roaring.MultiLog

	isSelf  ssb.Authorizer
	unboxer *private.Manager
}

func (g tangleHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink, edp muxrpc.Endpoint) error {
	if len(req.Args()) < 1 {
		return errors.Errorf("invalid arguments")
	}

	var qryarr []message.TanglesArgs
	var qry message.TanglesArgs

	err := json.Unmarshal(req.RawArgs, &qryarr)
	if err != nil {
		if req.RawArgs[0] != '"' {
			return errors.Wrap(err, "bad request - invalid root")
		}

		var ref refs.MessageRef
		err := json.Unmarshal(req.RawArgs, &ref)
		if err != nil {
			return errors.Wrap(err, "bad request - invalid root (string?)")
		}
		qry.Root = &ref
		qry.Limit = -1
		qry.Keys = true
	} else {
		if n := len(qryarr); n != 1 {
			return fmt.Errorf("expected 1 argument but got %d", n)
		}
		qry = qryarr[0]
		// defaults?!
	}

	if qry.Limit == 0 {
		qry.Limit = -1
	}

	remote, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		return errors.Wrap(err, "failed to determain remote")
	}

	isSelf := g.isSelf.Authorize(remote)
	if qry.Private && isSelf != nil {
		return fmt.Errorf("not authroized")
	}

	fmt.Println("query for:", qry.Root.Ref())
	spew.Dump(qry)

	// create toJSON sink
	lsnk := transform.NewKeyValueWrapper(snk, qry.Keys)

	// lookup address depending if we have a name for the tangle or not
	addr := librarian.Addr(append([]byte("v1:"), qry.Root.Hash...))
	if qry.Name != "" {
		addr = librarian.Addr("v2:"+qry.Name+":") + librarian.Addr(qry.Root.Hash)
	}

	// TODO: needs same kind of refactor that messagesByType needs

	if qry.Live {
		if qry.Private {
			return fmt.Errorf("TODO: fix live && private")
		}
		threadLog, err := g.tangles.Get(addr)
		if err != nil {
			return errors.Wrap(err, "failed to load thread")
		}

		src, err := mutil.Indirect(g.rxlog, threadLog).Query(margaret.Limit(int(qry.Limit)), margaret.Live(qry.Live), margaret.Reverse(qry.Reverse))
		if err != nil {
			return errors.Wrap(err, "tangle: failed to create query")
		}

		err = luigi.Pump(ctx, lsnk, src)
		if err != nil {
			return errors.Wrap(err, "tangle: failed to pump msgs")
		}

		return snk.Close()
	}

	// not live
	threadBmap, err := g.tangles.LoadInternalBitmap(addr)
	if err != nil {
		// TODO: check err == persist: not found
		return snk.Close()
		return errors.Wrap(err, "failed to load thread log")
	}

	if qry.Private {
		lsnk = g.unboxer.WrappedUnboxingSink(lsnk)
	} else {
		// filter all boxed messages from the stream
		box1, err := g.private.LoadInternalBitmap(librarian.Addr("meta:box1"))
		if err != nil {
			// TODO: compare not found
			// return errors.Wrap(err, "failed to load bmap for box1")
			box1 = bmap.New()
		}

		box2, err := g.private.LoadInternalBitmap(librarian.Addr("meta:box2"))
		if err != nil {
			// TODO: compare not found
			// return errors.Wrap(err, "failed to load bmap for box2")
			box2 = bmap.New()
		}

		box1.Or(box2) // all the boxed messages

		// remove all the boxed ones from the type we are looking up
		threadBmap.AndNot(box1)
	}

	// TODO: sort by previous

	it := threadBmap.Iterator()

	for it.HasNext() {
		seq := margaret.BaseSeq(it.Next())
		v, err := g.rxlog.Get(seq)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tangles failed to get seq:", seq, " with:", err)
			continue
		}

		// skip nulled
		if verr, ok := v.(error); ok && margaret.IsErrNulled(verr) {
			continue
		}

		if err := lsnk.Pour(ctx, v); err != nil {
			fmt.Fprintln(os.Stderr, "tangles failed send:", seq, " with:", err)
			break
		}

		if qry.Limit >= 0 {
			qry.Limit--
			if qry.Limit == 0 {
				break
			}
		}
	}

	return lsnk.Close()
}
