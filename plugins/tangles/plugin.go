// SPDX-License-Identifier: MIT

package tangles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	bmap "github.com/RoaringBitmap/roaring"
	"github.com/cryptix/go/encodedTime"
	"github.com/go-kit/kit/log"
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

func NewPlugin(getter ssb.Getter, rxlog margaret.Log, tangles, private *roaring.MultiLog, unboxer *private.Manager, isSelf ssb.Authorizer) *Plugin {
	mux := typemux.New(log.NewNopLogger())

	mux.RegisterSource(muxrpc.Method{"tangles", "replies"}, repliesHandler{
		getter:  getter,
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

type repliesHandler struct {
	getter ssb.Getter
	rxlog  margaret.Log

	tangles *roaring.MultiLog
	private *roaring.MultiLog

	isSelf  ssb.Authorizer
	unboxer *private.Manager
}

func (g repliesHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink) error {
	var qryarr []message.TanglesArgs
	var qry message.TanglesArgs

	err := json.Unmarshal(req.RawArgs, &qryarr)
	if err != nil {
		if req.RawArgs[0] != '"' {
			return fmt.Errorf("bad request - invalid root: %w", err)
		}

		var ref refs.MessageRef
		err := json.Unmarshal(req.RawArgs, &ref)
		if err != nil {
			return fmt.Errorf("bad request - invalid root (string?): %w", err)
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

	remote, err := ssb.GetFeedRefFromAddr(req.RemoteAddr())
	if err != nil {
		return fmt.Errorf("failed to determain remote: %w", err)
	}

	isSelf := g.isSelf.Authorize(remote)
	if qry.Private && isSelf != nil {
		return fmt.Errorf("not authroized")
	}

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
			return fmt.Errorf("failed to load thread: %w", err)
		}

		src, err := mutil.Indirect(g.rxlog, threadLog).Query(margaret.Limit(int(qry.Limit)), margaret.Live(qry.Live), margaret.Reverse(qry.Reverse))
		if err != nil {
			return fmt.Errorf("tangle: failed to create query: %w", err)
		}

		err = luigi.Pump(ctx, lsnk, src)
		if err != nil {
			return fmt.Errorf("tangle: failed to pump msgs: %w", err)
		}

		return snk.Close()
	}

	// not live
	threadBmap, err := g.tangles.LoadInternalBitmap(addr)
	if err != nil {
		// TODO: check err == persist: not found
		return snk.Close()
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

	// get root message
	var tps []refs.TangledPost
	root, err := g.getter.Get(*qry.Root)
	if err != nil {
		return err
	}
	var tp tangledPost
	err = json.Unmarshal(root.ContentBytes(), &tp.Value.Content)
	if err != nil {
		return fmt.Errorf("failed to unpack message %s: %w", root.Key().Ref(), err)
	}
	tp.TheKey = root.Key()
	tp.Value.Author = *root.Author()
	tp.Value.Sequence = margaret.BaseSeq(root.Seq())
	tp.Value.Timestamp = encodedTime.Millisecs(root.Claimed())
	tps = append(tps, tp)

	// get replies and add them to sorter
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

		msg, ok := v.(refs.Message)
		if !ok {
			return fmt.Errorf("not a mesg %T", v)
		}

		// TODO: unbox private messages

		// find tangles
		var tp tangledPost
		err = json.Unmarshal(msg.ContentBytes(), &tp.Value.Content)
		if err != nil {
			return fmt.Errorf("failed to unpack message %s: %w", msg.Key().Ref(), err)
		}
		tp.TheKey = msg.Key()
		tp.Value.Author = *msg.Author()
		tp.Value.Sequence = margaret.BaseSeq(msg.Seq())
		tp.Value.Timestamp = encodedTime.Millisecs(msg.Claimed())

		tps = append(tps, tp)

		if qry.Limit >= 0 {
			qry.Limit--
			if qry.Limit == 0 {
				break
			}
		}
	}

	// sort them
	sorter := refs.ByPrevious{Items: tps, TangleName: ""}
	sorter.FillLookup()
	sort.Sort(sorter)

	// stream them out
	enc := json.NewEncoder(snk)
	snk.SetEncoding(muxrpc.TypeJSON)
	for i, p := range sorter.Items {
		if qry.Keys {
			err = enc.Encode(p)
		} else {
			err = enc.Encode(p.(tangledPost).Value)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "tangles failed send:", i, " with:", err)
			break
		}
	}

	// return lsnk.Close()
	return snk.Close()
}

type tangledPost struct {
	TheKey *refs.MessageRef `json:"key"`
	Value  struct {
		refs.Value
		// substitute Content with refs.Post
		Content refs.Post `json:"content"`
	} `json:"value"`
}

func (tm tangledPost) Key() *refs.MessageRef {
	return tm.TheKey
}

func (tm tangledPost) Tangle(name string) (*refs.MessageRef, refs.MessageRefs) {
	if name == "" {
		return tm.Value.Content.Root, tm.Value.Content.Branch
	}

	tp, has := tm.Value.Content.Tangles[name]
	if !has {
		return nil, nil
	}

	return tp.Root, tp.Previous
}
