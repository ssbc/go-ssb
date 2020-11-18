package muxmux

import (
	"context"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
)

type SourceFunc func(context.Context, *muxrpc.Request, luigi.Sink) error

func (sf SourceFunc) HandleSource(ctx context.Context, r *muxrpc.Request, snk luigi.Sink) error {
	return sf(ctx, r, snk)
}

// SourceHandler initiates a 'source' call, so the handler is supposed to send a stream of stuff to the peer.
type SourceHandler interface {
	HandleSource(context.Context, *muxrpc.Request, luigi.Sink, muxrpc.Endpoint) error
}

type sourceStub struct {
	h SourceHandler
}

func (hm sourceStub) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: check call type

	err := hm.h.HandleSource(ctx, req, req.Stream, edp)
	if err != nil {
		req.CloseWithError(err)
		return
	}
}

func (hm sourceStub) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}
