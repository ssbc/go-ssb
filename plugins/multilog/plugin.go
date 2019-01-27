package multilog

import (
	"context"

	"github.com/pkg/errors"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb/multilogs"
)

type Plugin struct {
	mgr multilogs.Manager
}

func (*Plugin) Name() string          { return "multilogs" }
func (*Plugin) Method() muxrpc.Method { return muxrpc.Method{"multilog"} }

func (p *Plugin) Handler() muxrpc.Handler {
	var mux muxrpc.HandlerMux

	ufHandler := newMultilogHandler("feeds", p.mgr.UserFeeds())
	mtHandler := newMultilogHandler("types", p.mgr.MessageTypes())

	mux.Register(ufHandler.Method, ufHandler)
	mux.Register(ufHandler.Method, mtHandler)

	return &mux
}

func newMultilogHandler(name string, mlog multilog.MultiLog) methodHandler {
	var mux muxrpc.HandlerMux

	mux.Register(muxrpc.Method{"multilog", name, "get"}, getHandler{mlog})

	return methodHandler{&mux, muxrpc.Method{"multilog", name}}
}

type getHandler struct {
	mlog multilog.MultiLog
}

func (getHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h getHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args) != 1 {
		req.CloseWithError(errors.New("command expects one argument (sublog key)"))
		return
	}

	str, ok := req.Args[0].(string)
	if !ok {
		req.CloseWithError(errors.Errorf("expected argument to be of type %T, got %T", str, req.Args[0]))
		return
	}

	seq, ok := req.Args[1].(int)
	if !ok {
		req.CloseWithError(errors.Errorf("expected argument to be of type %T, got %T", str, req.Args[0]))
		return
	}

	sublog, err := h.mlog.Get(librarian.Addr(str))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "could not get sublog"))
		return
	}

	v, err := sublog.Get(margaret.BaseSeq(seq))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "could not get value"))
		return
	}

	req.Return(ctx, v)
}

type methodHandler struct {
	muxrpc.Handler

	Method muxrpc.Method
}
