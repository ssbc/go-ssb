package rawread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.mindeco.de/log"

	"go.cryptoscope.co/muxrpc/v2/typemux"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

// ~> sbot createFeedStream --help
// (log) Fetch messages ordered by the time received.
// log [--live] [--gt ts] [--lt ts] [--reverse]  [--keys] [--limit n]
type sortedPlug struct {
	root margaret.Log
	res  *repo.SequenceResolver

	h muxrpc.Handler
}

func NewSortedStream(log log.Logger, rootLog margaret.Log, res *repo.SequenceResolver) ssb.Plugin {
	plug := &sortedPlug{
		root: rootLog,
		res:  res,
	}

	h := typemux.New(log)

	h.RegisterSource(muxrpc.Method{"createFeedStream"}, plug)

	plug.h = &h

	return plug
}

func (lt sortedPlug) Name() string            { return "createFeedStream" }
func (sortedPlug) Method() muxrpc.Method      { return muxrpc.Method{"createFeedStream"} }
func (lt sortedPlug) Handler() muxrpc.Handler { return lt.h }

func (g sortedPlug) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink) error {
	start := time.Now()
	var qry message.CreateLogArgs
	if len(req.Args()) == 1 {
		var args []message.CreateLogArgs
		err := json.Unmarshal(req.RawArgs, &args)
		if err != nil {
			return fmt.Errorf("bad request data: %w", err)
		}
		if len(args) == 1 {
			qry = args[0]
		}
	} else {
		// Defaults for no arguments
		qry.Keys = true
		qry.Limit = -1
	}

	// empty query doesn't make much sense...
	if qry.Limit == 0 {
		qry.Limit = -1
	}

	// TODO: only return message keys
	// qry.Values = true

	sortedSeqs, err := g.res.SortAndFilterAll(repo.SortByClaimed, func(ts int64) bool {
		isGreater := ts > qry.Gt
		isSmaller := ts < qry.Lt
		return isGreater && isSmaller
	}, true)

	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "createFeedStream sorted seqs:", len(sortedSeqs), "after:", time.Since(start))

	toJSON := transform.NewKeyValueWrapper(snk, qry.Keys)

	for _, res := range sortedSeqs {
		v, err := g.root.Get(margaret.BaseSeq(res.Seq))
		if err != nil {
			fmt.Fprintln(os.Stderr, "createFeedStream failed to get seq:", res.Seq, " with:", err, "after:", time.Since(start))
			continue
		}

		if err := toJSON.Pour(ctx, v); err != nil {
			fmt.Fprintln(os.Stderr, "createFeedStream failed send:", res.Seq, " with:", err, "after:", time.Since(start))
			break
		}

		if qry.Limit >= 0 {
			qry.Limit--
			if qry.Limit == 0 {
				break
			}
		}
	}
	fmt.Fprintln(os.Stderr, "createFeedStream closed after:", time.Since(start))
	return snk.Close()
}
