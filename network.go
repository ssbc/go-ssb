package ssb

import (
	"context"
	"io"
	"net"
	"time"

	"go.cryptoscope.co/muxrpc"
)

type Network interface {
	Connect(ctx context.Context, addr net.Addr) error
	Serve(context.Context, ...muxrpc.HandlerWrapper) error
	GetListenAddr() net.Addr

	GetEndpointFor(*FeedRef) (muxrpc.Endpoint, bool)

	GetConnTracker() ConnTracker

	io.Closer
}

type ConnTracker interface {
	Active(net.Addr) bool
	OnAccept(conn net.Conn) bool
	OnClose(conn net.Conn) time.Duration
	Count() uint
	CloseAll()
}
