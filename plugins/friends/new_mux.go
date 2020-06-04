package friends

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
)

type AsyncHandler interface {
	HandleAsync(context.Context, *muxrpc.Request) (interface{}, error)
}

// SourceHandler initiates a 'source' call, so the handler is supposed to send a stream of stuff to the peer.
type SourceHandler interface {
	HandleSource(context.Context, *muxrpc.Request, luigi.Sink) error
}

// SinkHandler initiates a 'sink' call. The handler receives stuff from the peer through the passed source
type SinkHandler interface {
	HandleSource(context.Context, *muxrpc.Request, luigi.Source) error
}

type DuplexHandler interface {
	HandleSource(context.Context, *muxrpc.Request, luigi.Source, luigi.Sink) error
}

type HandlerMux struct {
	logger log.Logger

	handlers map[string]muxrpc.Handler
}

func (hm *HandlerMux) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	for i := len(req.Method); i > 0; i-- {
		m := req.Method[:i]
		h, ok := hm.handlers[m.String()]
		if ok {
			h.HandleCall(ctx, req, edp)
			return
		}
	}

	req.Stream.CloseWithError(errors.Errorf("no such command: %v", req.Method))
}

// HandleConnect does nothing on this mux since it's only intended for function calls, not connect events
func (hm *HandlerMux) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

// RegisterAsync registers a 'async' call for name method
func (hm *HandlerMux) RegisterAsync(m muxrpc.Method, h AsyncHandler) {
	if hm.handlers == nil {
		hm.handlers = make(map[string]muxrpc.Handler)
	}

	hm.handlers[m.String()] = asyncStub{
		logger: hm.logger,
		h:      h,
	}
}

// RegisterSource registers a 'source' call for name method
func (hm *HandlerMux) RegisterSource(m muxrpc.Method, h SourceHandler) {
	if hm.handlers == nil {
		hm.handlers = make(map[string]muxrpc.Handler)
	}

	hm.handlers[m.String()] = sourceStub{
		logger: hm.logger,
		h:      h,
	}
}

type asyncStub struct {
	logger log.Logger

	h AsyncHandler
}

func (hm asyncStub) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: check call type

	v, err := hm.h.HandleAsync(ctx, req)
	if err != nil {
		req.CloseWithError(err)
		return
	}

	err = req.Return(ctx, v)
	if err != nil {
		level.Error(hm.logger).Log("evt", "return failed", "err", err, "method", req.Method.String())
	}
}

func (hm asyncStub) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

type sourceStub struct {
	logger log.Logger

	h SourceHandler
}

func (hm sourceStub) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: check call type

	err := hm.h.HandleSource(ctx, req, req.Stream)
	if err != nil {
		req.CloseWithError(err)
		return
	}

	//	req.Stream.Close()
}

func (hm sourceStub) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}
