package ebt

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
)

type handler struct {
	info   logging.Interface
	isServ bool

	id           *refs.FeedRef
	rootLog      margaret.Log
	userFeeds    multilog.MultiLog
	graphBuilder graph.Builder
}

func New(i logging.Interface, id *refs.FeedRef, rootLog margaret.Log, userFeeds multilog.MultiLog, graphBuilder graph.Builder) muxrpc.Handler {
	return &handler{
		info:         i,
		id:           id,
		rootLog:      rootLog,
		userFeeds:    userFeeds,
		graphBuilder: graphBuilder,
	}
}

func (h *handler) check(err error) {
	if err != nil {
		h.info.Log("error", err)
	}
}

// server side
func (h *handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	// TODO: find a way to signal if we are client or server

	// ref, err := ssb.GetFeedRefFromAddr(e.Remote())
	// if err != nil {
	// 	h.info.Log("event", "call replicate", "err", err)
	// 	// checkAndClose(err)
	// 	return
	// }

	// var opt = map[string]interface{}{
	// 	"version": 3,
	// }
	// var msgs interface{}
	// src, snk, err := e.Duplex(ctx, msgs, muxrpc.Method{"ebt", "replicate"}, opt)
	// h.info.Log("event", "call replicate", "err", err)
	// if err != nil {
	// 	return
	// }
	// go h.pump(ctx, snk, ref)
	// h.drain(ctx, src)
	// // snk.Close()
	// h.info.Log("event", "call replicate", "err", err)
}

// client side (getting called by server asking for support)
func (h *handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {

	var closed bool
	checkAndClose := func(err error) {
		h.check(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			h.check(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	defer func() {
		if !closed {
			h.check(errors.Wrapf(req.Stream.Close(), "gossip: error closing call: %s", req.Method))
		}
	}()

	if req.Method.String() != "ebt.replicate" {
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
		return
	}

	if req.Type != "duplex" {
		checkAndClose(errors.Errorf("invalid type: %s", req.Type))
		return
	}

	h.info.Log("debug", "called", "args", fmt.Sprintf("%+v", req.Args))

	err := h.sendState(ctx, req.Stream)
	if err != nil {
		h.info.Log("event", "sendState failed", "err", err)
		checkAndClose(err)
		return
	}

	req.Stream.CloseWithError(fmt.Errorf("sorry:unsupported"))

	ref, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		checkAndClose(err)
		return
	}

	v, err := req.Stream.Next(ctx)
	if err != nil {
		if luigi.IsEOS(err) {
			return
		}
		h.info.Log("err", err, "msg", "did not get ebt state")
		return
	}
	// h.info.Log("debug", "got ebt state", "type", fmt.Sprintf("%T", v))
	// goon.Dump(v)

	b, err := json.Marshal(v)
	if err != nil {
		h.info.Log("err", err, "msg", "json dump failed state")
		return
	}
	ioutil.WriteFile(fmt.Sprintf("ebt-%x.json", ref.ID), b, 0700)
}

func (h handler) sendState(ctx context.Context, snk luigi.Sink) error {
	// current state:
	currState := make(map[string]int64)

	fs := h.graphBuilder.Hops(h.id, 2)
	if fs != nil {
		hopList, err := fs.List()
		if err != nil {
			return errors.Wrap(err, "hops set listing failed")
		}
		for _, ref := range hopList {
			currState[ref.Ref()] = -1
		}
	}

	addrs, err := h.userFeeds.List()
	if err != nil {
		return errors.Wrap(err, "failed to get userlist")
	}
	for _, addr := range addrs {
		// l, err := h.userFeeds.Get(addr)
		// if err != nil {
		// 	return errors.Wrapf(err, "failed to get user log:%d", i)
		// }
		// sv, err := l.Seq().Value()
		// if err != nil {
		// 	return errors.Wrapf(err, "failed to get sequence for user log:%d", i)
		// }
		ref := &refs.FeedRef{
			ID:   []byte(addr),
			Algo: refs.RefAlgoFeedSSB1,
		}
		currState[ref.Ref()] = -1 // (sv.(margaret.Seq).Seq() + 1)
	}

	// b, err := json.Marshal(currState)
	// if err != nil {
	// 	h.info.Log("err", err, "msg", "json dump failed state")
	// } else {
	// 	ioutil.WriteFile(fmt.Sprintf("ebt-%x.json", h.id), b, 0700)
	// }

	err = snk.Pour(ctx, currState)
	if err != nil {
		return errors.Wrapf(err, "failed to send currState: %d", len(currState))
	}

	// mylog, err := h.userFeeds.Get(librarian.Addr(h.id.ID))
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to open mylog")
	// }

	// src, err := mylog.Query()
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to open mylog")
	// }

	// err = luigi.Pump(ctx, snk, src)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to copy mylog to remote")
	// }
	time.Sleep(2 * time.Second)

	return nil
}
