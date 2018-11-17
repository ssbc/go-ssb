package ssb

import (
	"context"
	"fmt"
	"net"

	"github.com/agl/ed25519"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
)

type Options struct {
	ListenAddr   net.Addr
	KeyPair      *KeyPair
	AppKey       []byte
	MakeHandler  func(net.Conn) (muxrpc.Handler, error)
	Logger       log.Logger
	ConnWrappers []netwrap.ConnWrapper

	EventCounter    *prometheus.Counter
	SystemGauge     *prometheus.Gauge
	EndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint
}

type Node interface {
	Connect(ctx context.Context, addr net.Addr) error
	Serve(ctx context.Context) error
	GetListenAddr() net.Addr
}

type node struct {
	opts Options

	l            net.Listener
	secretServer *secretstream.Server
	secretClient *secretstream.Client
	connTracker  ConnTracker
	log          log.Logger
	connWrappers []netwrap.ConnWrapper

	edpWrapper func(muxrpc.Endpoint) muxrpc.Endpoint
	evtCtr     *prometheus.Counter
	sysGauge   *prometheus.Gauge
}

func NewNode(opts Options) (Node, error) {
	n := &node{
		opts:        opts,
		connTracker: NewConnTracker(),
	}

	var err error

	n.secretClient, err = secretstream.NewClient(opts.KeyPair.Pair, opts.AppKey)
	if err != nil {
		return nil, errors.Wrap(err, "error creating secretstream.Client")
	}

	n.secretServer, err = secretstream.NewServer(opts.KeyPair.Pair, opts.AppKey)
	if err != nil {
		return nil, errors.Wrap(err, "error creating secretstream.Server")
	}

	n.l, err = netwrap.Listen(n.opts.ListenAddr, n.secretServer.ListenerWrapper())
	if err != nil {
		return nil, errors.Wrap(err, "error creating listener")
	}

	n.connWrappers = opts.ConnWrappers

	n.edpWrapper = opts.EndpointWrapper
	n.evtCtr = opts.EventCounter
	n.sysGauge = opts.SystemGauge

	if n.sysGauge != nil {
		n.sysGauge.With("part", "conns").Set(0)
		n.sysGauge.With("part", "fetches").Set(0)
	}
	n.log = opts.Logger

	return n, nil
}

type ErrOutOfReach struct {
	Dist int
	Max  int
}

func (e ErrOutOfReach) Error() string {
	return fmt.Sprintf("sbot: peer not in reach. d:%d, max:%d", e.Dist, e.Max)
}

func (n *node) handleConnection(ctx context.Context, conn net.Conn, hws ...muxrpc.HandlerWrapper) {
	n.connTracker.OnAccept(conn)
	defer n.connTracker.OnClose(conn)
	defer func() {
		if n.sysGauge != nil {
			n.sysGauge.With("part", "conns").Add(-1)
		}
		conn.Close()
	}()

	if n.evtCtr != nil {
		n.evtCtr.With("event", "connection").Add(1)
	}

	h, err := n.opts.MakeHandler(conn)
	if err != nil {
		if _, ok := errors.Cause(err).(*ErrOutOfReach); ok {
			return // ignore silently
		}
		n.log.Log("func", "handleConnection", "op", "MakeHandler", "error", err.Error(), "peer", conn.RemoteAddr())
		return
	}

	pkr := muxrpc.NewPacker(conn)
	edp := muxrpc.HandleWithRemote(pkr, h, conn.RemoteAddr())

	if n.edpWrapper != nil {
		edp = n.edpWrapper(edp)
	}

	srv := edp.(muxrpc.Server)
	if err := srv.Serve(ctx); err != nil {
		n.log.Log("func", "handleConnection", "op", "Serve", "error", err.Error(), "peer", conn.RemoteAddr())
	}
}

func (n *node) Serve(ctx context.Context) error {
	for {
		conn, err := n.l.Accept()
		if err != nil {
			return errors.Wrap(err, "error accepting connection")
		}
		if n.sysGauge != nil {
			n.sysGauge.With("part", "conns").Add(1)
		}

		// apply connection wrappers
		for i, cw := range n.connWrappers {
			var err error
			conn, err = cw(conn)
			if err != nil {
				return errors.Wrapf(err, "error applying connection wrapper #%d", i)
			}
			n.log.Log("action", "applied connection wrapper", "i", i, "count", len(n.connWrappers))
		}

		go func(c net.Conn) {
			n.handleConnection(ctx, c)
		}(conn)
	}
}

func (n *node) Connect(ctx context.Context, addr net.Addr) error {
	shsAddr := netwrap.GetAddr(addr, "shs-bs")
	if shsAddr == nil {
		return errors.New("expected an address containing an shs-bs addr")
	}

	var pubKey [ed25519.PublicKeySize]byte
	if shsAddr, ok := shsAddr.(secretstream.Addr); ok {
		copy(pubKey[:], shsAddr.PubKey)
	} else {
		return errors.New("expected shs-bs address to be of type secretstream.Addr")
	}

	conn, err := netwrap.Dial(netwrap.GetAddr(addr, "tcp"), n.secretClient.ConnWrapper(pubKey))
	if err != nil {
		return errors.Wrap(err, "error dialing")
	}

	if cnt := len(n.connWrappers); cnt > 0 {
		n.log.Log("action", "applying connection wrappers", "count", cnt)
	}
	// apply connection wrappers
	for i, cw := range n.connWrappers {
		var err error
		conn, err = cw(conn)
		if err != nil {
			return errors.Wrapf(err, "error applying connection wrapper #%d", i)
		}
	}

	if n.sysGauge != nil {
		n.sysGauge.With("part", "conns").Add(1)
	}

	go func(c net.Conn) {
		n.handleConnection(ctx, c)
	}(conn)
	return nil
}

func (n *node) GetListenAddr() net.Addr {
	return n.l.Addr()
}
