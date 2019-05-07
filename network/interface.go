package network

import (
	"context"
	"io"
	"net"

	"go.cryptoscope.co/muxrpc"
)

type Interface interface {
	Connect(ctx context.Context, addr net.Addr) error
	Serve(context.Context, ...muxrpc.HandlerWrapper) error
	GetListenAddr() net.Addr

	GetConnTracker() ConnTracker

	io.Closer
}
