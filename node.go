package ssb

import (
	"context"
	"fmt"
	"net"
	"time"

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
	Latency         *prometheus.Summary
	EndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint
}

type Node interface {
	Connect(ctx context.Context, addr net.Addr) error
	Serve(context.Context, ...muxrpc.HandlerWrapper) error
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
	latency    *prometheus.Summary
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
	n.latency = opts.Latency

	if n.sysGauge != nil {
		n.sysGauge.With("part", "conns").Set(0)
		n.sysGauge.With("part", "fetches").Set(0)

		n.connTracker = NewInstrumentedConnTracker(n.connTracker, n.sysGauge, n.latency)
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
	ok := n.connTracker.OnAccept(conn)
	if !ok {
		err := conn.Close()
		n.log.Log("conn", "ignored", "remote", conn.RemoteAddr(), "err", err)
		return
	}
	var pkr muxrpc.Packer

	var closed bool
	defer func() {
		closed = true
		durr := n.connTracker.OnClose(conn)
		var err error
		if pkr != nil {
			err = errors.Wrap(pkr.Close(), "packer closing")
		} else {
			err = errors.Wrap(conn.Close(), "direct conn closing")
		}
		n.log.Log("conn", "closing", "err", err, "durr", fmt.Sprintf("%v", durr))
	}()

	if n.evtCtr != nil {
		n.evtCtr.With("event", "connection").Add(1)
	}

	h, err := n.opts.MakeHandler(conn)
	if err != nil {
		if _, ok := errors.Cause(err).(*ErrOutOfReach); ok {
			return // ignore silently
		}
		n.log.Log("conn", "mkHandler", "err", err, "peer", conn.RemoteAddr())
		return
	}

	for _, hw := range hws {
		h = hw(h)
	}

	pkr = muxrpc.NewPacker(conn)
	edp := muxrpc.HandleWithRemote(pkr, h, conn.RemoteAddr())

	if n.edpWrapper != nil {
		edp = n.edpWrapper(edp)
	}

	go func() {
		time.Sleep(15 * time.Minute)
		if closed {
			return
		}
		n.log.Log("sorry", "overdue bug")
		err := pkr.Close()
		n.log.Log("conn", "long close", "err", err, "connErr", conn.Close())
		n.connTracker.OnClose(conn)
		pkr = nil
	}()

	srv := edp.(muxrpc.Server)
	if err := srv.Serve(ctx); err != nil {
		n.log.Log("conn", "serve", "err", err, "peer", conn.RemoteAddr())
	}
}

func (n *node) Serve(ctx context.Context, wrappers ...muxrpc.HandlerWrapper) error {
	for {
		conn, err := n.l.Accept()
		if err != nil {
			n.log.Log("msg", "node/Serve: failed to accepting connection", "err", err)
			continue
		}

		conn, err = n.applyConnWrappers(conn)
		if err != nil {
			n.log.Log("msg", "node/Serve: failed to wrap connection", "err", err)
			continue
		}

		go func(c net.Conn) {
			n.handleConnection(ctx, c, wrappers...)
		}(conn)
	}
}

func (n *node) Connect(ctx context.Context, addr net.Addr) error {
	shsAddr := netwrap.GetAddr(addr, "shs-bs")
	if shsAddr == nil {
		return errors.New("node/connect: expected an address containing an shs-bs addr")
	}

	var pubKey [ed25519.PublicKeySize]byte
	if shsAddr, ok := shsAddr.(secretstream.Addr); ok {
		copy(pubKey[:], shsAddr.PubKey)
	} else {
		return errors.New("node/connect: expected shs-bs address to be of type secretstream.Addr")
	}

	conn, err := netwrap.Dial(netwrap.GetAddr(addr, "tcp"), n.secretClient.ConnWrapper(pubKey))
	if err != nil {
		return errors.Wrap(err, "node/connect: error dialing")
	}

	conn, err = n.applyConnWrappers(conn)
	if err != nil {
		return errors.Wrap(err, "node/connect: wrap failed")
	}

	go func(c net.Conn) {
		n.handleConnection(ctx, c)
	}(conn)
	return nil
}

func (n *node) GetListenAddr() net.Addr {
	return n.l.Addr()
}

func (n *node) applyConnWrappers(conn net.Conn) (net.Conn, error) {
	for i, cw := range n.connWrappers {
		var err error
		conn, err = cw(conn)
		if err != nil {
			return nil, errors.Wrapf(err, "error applying connection wrapper #%d", i)
		}
	}
	return conn, nil
}
