package ssb

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
)

// DefaultPort is the default listening port for ScuttleButt.
const DefaultPort = 8008

type Options struct {
	Dialer        netwrap.Dialer
	ListenAddr    net.Addr
	EnableAdverts bool

	KeyPair      *KeyPair
	AppKey       []byte
	MakeHandler  func(net.Conn) (muxrpc.Handler, error)
	Logger       log.Logger
	ConnWrappers []netwrap.ConnWrapper

	EventCounter    *prometheus.Counter
	SystemGauge     *prometheus.Gauge
	Latency         *prometheus.Summary
	EndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint
}

type Node interface {
	Connect(ctx context.Context, addr net.Addr) error
	Serve(context.Context, ...muxrpc.HandlerWrapper) error
	GetListenAddr() net.Addr

	GetConnTracker() ConnTracker

	io.Closer
}

var ErrShuttingDown = errors.Errorf("ssb: shutting down now") // this is fine

type ErrOutOfReach struct {
	Dist int
	Max  int
}

func (e ErrOutOfReach) Error() string {
	return fmt.Sprintf("ssb/graph: peer not in reach. d:%d, max:%d", e.Dist, e.Max)
}
