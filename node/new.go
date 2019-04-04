package node

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/agl/ed25519"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
)

type node struct {
	opts ssb.Options

	dialer            netwrap.Dialer
	l                 net.Listener
	networkAdvertiser *Advertiser
	secretServer      *secretstream.Server
	secretClient      *secretstream.Client
	connTracker       ssb.ConnTracker
	log               log.Logger
	connWrappers      []netwrap.ConnWrapper

	edpWrapper func(muxrpc.Endpoint) muxrpc.Endpoint
	evtCtr     *prometheus.Counter
	sysGauge   *prometheus.Gauge
	latency    *prometheus.Summary
}

func New(opts ssb.Options) (ssb.Node, error) {
	n := &node{
		opts:        opts,
		connTracker: ssb.NewConnTracker(),
	}

	var err error

	if opts.Dialer != nil {
		n.dialer = opts.Dialer
	} else {
		n.dialer = netwrap.Dial
	}

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

	if n.opts.EnableAdverts {
		n.networkAdvertiser, err = NewAdvertiser(n.opts.ListenAddr, opts.KeyPair)
		if err != nil {
			return nil, errors.Wrap(err, "error creating Advertiser")
		}
	}

	n.connWrappers = opts.ConnWrappers

	n.edpWrapper = opts.EndpointWrapper
	n.evtCtr = opts.EventCounter
	n.sysGauge = opts.SystemGauge
	n.latency = opts.Latency

	if n.sysGauge != nil {
		n.sysGauge.With("part", "conns").Set(0)
		n.sysGauge.With("part", "fetches").Set(0)

		n.connTracker = ssb.NewInstrumentedConnTracker(n.connTracker, n.sysGauge, n.latency)
	}
	n.log = opts.Logger

	return n, nil
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
		if _, ok := errors.Cause(err).(*ssb.ErrOutOfReach); ok {
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

	ctx, cancel := context.WithTimeout(ctx, time.Minute*15)
	go func() {
		defer cancel()
		time.Sleep(15 * time.Minute)
		if closed {
			return
		}
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
	if n.networkAdvertiser != nil {
		err := n.networkAdvertiser.Start()
		if err == nil {
			// should also be called when ctx canceled?
			// or pass ctx to adv start?
			defer n.networkAdvertiser.Stop()

			go func() {
				ch, done := n.networkAdvertiser.Notify()
				defer done() // might trigger close of closed panic
				for a := range ch {
					if n.connTracker.Active(a) {
						continue
					}
					if err := n.Connect(ctx, a); err != nil {
						n.log.Log("event", "debug", "msg", "advert dialback failed", "err", err)
					}
				}
			}()
		}
		n.log.Log("event", "advstart failed", "err", err)
	}

	for {
		conn, err := n.l.Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				// yikes way of handling this
				// but means this needs to be restarted anyway
				return nil
			}
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

	conn, err := n.dialer(netwrap.GetAddr(addr, "tcp"), n.secretClient.ConnWrapper(pubKey))
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

func (n *node) GetConnTracker() ssb.ConnTracker {
	return n.connTracker
}

func (n *node) Close() error {
	err := n.l.Close()
	if err != nil {
		return errors.Wrap(err, "ssb: network node failed to close it's listener")
	}
	if cnt := n.connTracker.Count(); cnt > 0 {
		n.log.Log("event", "warning", "msg", "still open connections", "count", cnt)
		n.connTracker.CloseAll()
	}
	return nil
}
