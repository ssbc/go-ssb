package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mitchellh/mapstructure"

	"cryptoscope.co/go/librarian"
	"cryptoscope.co/go/margaret"
	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/sbot"
	"cryptoscope.co/go/sbot/message"
	"github.com/pkg/errors"
)

type createHistStream struct {
	I    sbot.FeedRef
	Repo sbot.Repo

	// ugly hack
	running sync.Mutex
}

func (createHistStream) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

func (hist createHistStream) HandleCall(ctx context.Context, req *muxrpc.Request) {
	hist.running.Lock()
	defer hist.running.Unlock()

	var closed bool
	checkAndClose := func(err error) {
		checkAndLog(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			checkAndLog(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	defer func() {
		if !closed {
			checkAndLog(errors.Wrapf(req.Stream.Close(), "createHistStream: error closing call: %s", req.Method))
		}
	}()

	qv := req.Args[0].(map[string]interface{})
	var qry message.CreateHistArgs
	err := mapstructure.Decode(qv, &qry)
	if err != nil {
		checkAndClose(errors.Wrap(err, "failed to decode qry map"))
		return
	}

	ref, err := sbot.ParseRef(qry.Id)
	if err != nil {
		checkAndClose(errors.Wrap(err, "illegal ref"))
		return
	}

	if err := hist.pushFeed(ctx, req, ref, qry); err != nil {
		checkAndClose(errors.Wrapf(err, "failed to push feed %s", ref.Ref()))
		return
	}
	checkAndLog(req.Stream.Close())
	return
}

func (hist *createHistStream) pushFeed(ctx context.Context, req *muxrpc.Request, ref sbot.Ref, qry message.CreateHistArgs) error {
	latestObv, err := hist.Repo.GossipIndex().Get(ctx, librarian.Addr(fmt.Sprintf("latest:%s", ref.Ref())))
	if err != nil {
		return errors.Wrap(err, "failed to get latest")
	}

	latestV, err := latestObv.Value()
	if err != nil {
		return errors.Wrap(err, "failed to observ latest")
	}

	switch v := latestV.(type) {
	case librarian.UnsetValue:
	case margaret.Seq:
		if qry.Seq >= v {
			return nil
		}

		fr := ref.(*sbot.FeedRef)
		seqs, err := hist.Repo.FeedSeqs(*fr)
		if err != nil {
			return errors.Wrap(err, "failed to get internal seqs")
		}
		rest := seqs[qry.Seq-1:]
		if len(rest) > 500 { // batch - slow but steady
			rest = rest[:150]
		}
		log := hist.Repo.Log()
		for i, rSeq := range rest {
			v, err := log.Get(rSeq)
			if err != nil {
				return errors.Wrapf(err, "load message %d", i)
			}
			stMsg := v.(message.StoredMessage)

			type fuck struct{ json.RawMessage }
			if err := req.Stream.Pour(ctx, fuck{stMsg.Raw}); err != nil {
				return errors.Wrap(err, "failed to pour msg to remote peer")

			}
		}
		fmt.Println("sent:", ref.Ref(), "from", qry.Seq, "rest:", len(rest))
	}
	return nil
}
