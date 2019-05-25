package network

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/agl/ed25519"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/secretstream/secrethandshake"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/ctxutils"
)

// DefaultPort is the default listening port for ScuttleButt.
const DefaultPort = 8008

type Options struct {
	Dialer     netwrap.Dialer
	ListenAddr net.Addr

	AdvertsSend      bool
	AdvertsConnectTo bool

	KeyPair      *ssb.KeyPair
	AppKey       []byte
	MakeHandler  func(net.Conn) (muxrpc.Handler, error)
	Logger       log.Logger
	ConnWrappers []netwrap.ConnWrapper

	EventCounter    *prometheus.Counter
	SystemGauge     *prometheus.Gauge
	Latency         *prometheus.Summary
	EndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint
}

type node struct {
	opts Options

	dialer        netwrap.Dialer
	l             net.Listener
	localDiscovRx *Discoverer
	localDiscovTx *Advertiser
	secretServer  *secretstream.Server
	secretClient  *secretstream.Client
	connTracker   ssb.ConnTracker
	log           log.Logger
	connWrappers  []netwrap.ConnWrapper

	remotesLock sync.Mutex
	remotes     map[string]muxrpc.Endpoint

	edpWrapper func(muxrpc.Endpoint) muxrpc.Endpoint
	evtCtr     *prometheus.Counter
	sysGauge   *prometheus.Gauge
	latency    *prometheus.Summary
}

func New(opts Options) (ssb.Network, error) {
	n := &node{
		opts: opts,
		// TODO: make this configurable
		// TODO: make multiple listeners (localhost:8008 should not restrict or kill connections)
		connTracker: NewLastWinsTracker(),

		remotes: make(map[string]muxrpc.Endpoint),
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

	if n.opts.AdvertsSend {
		n.localDiscovTx, err = NewAdvertiser(n.opts.ListenAddr, opts.KeyPair)
		if err != nil {
			return nil, errors.Wrap(err, "error creating Advertiser")
		}
	}

	if n.opts.AdvertsConnectTo {
		n.localDiscovRx, err = NewDiscoverer(opts.KeyPair)
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

		n.connTracker = NewInstrumentedConnTracker(n.connTracker, n.sysGauge, n.latency)
	}
	n.log = opts.Logger

	return n, nil
}

func (n *node) handleConnection(ctx context.Context, conn net.Conn, hws ...muxrpc.HandlerWrapper) {
	conn, err := n.applyConnWrappers(conn)
	if err != nil {
		conn.Close()
		n.log.Log("msg", "node/Serve: failed to wrap connection", "err", err)
		return
	}

	ok := n.connTracker.OnAccept(conn)
	if !ok {
		err := conn.Close()
		n.log.Log("conn", "ignored", "remote", conn.RemoteAddr(), "err", err)
		return
	}
	var pkr muxrpc.Packer

	ctx, cancel := ctxutils.WithError(ctx, fmt.Errorf("handle conn returned"))

	defer func() {
		durr := n.connTracker.OnClose(conn)
		var err error
		if pkr != nil {
			err = errors.Wrap(pkr.Close(), "packer closing")
		} else {
			err = errors.Wrap(conn.Close(), "direct conn closing")
		}
		if err != nil {
			n.log.Log("conn", "closing", "err", err, "durr", fmt.Sprintf("%v", durr))
		}
		cancel()
	}()

	if n.evtCtr != nil {
		n.evtCtr.With("event", "connection").Add(1)
	}

	h, err := n.opts.MakeHandler(conn)
	if err != nil {
		n.log.Log("conn", "mkHandler", "err", err, "peer", conn.RemoteAddr())
		if _, ok := errors.Cause(err).(*ssb.ErrOutOfReach); ok {
			return // ignore silently
		}
		return
	}

	for _, hw := range hws {
		h = hw(h)
	}

	pkr = muxrpc.NewPacker(conn)
	edp := muxrpc.HandleWithRemote(pkr, h, conn.RemoteAddr())

	if cn, ok := pkr.(muxrpc.CloseNotifier); ok {
		go func() {
			<-cn.Closed()
			cancel()
		}()
	}

	if n.edpWrapper != nil {
		edp = n.edpWrapper(edp)
	}

	srv := edp.(muxrpc.Server)

	n.addRemote(edp)
	if err := srv.Serve(ctx); err != nil {
		n.log.Log("conn", "serve", "err", err, "peer", conn.RemoteAddr())
	}
	n.removeRemote(edp)
}

// GetEndpointFor returns a muxrpc endpoint to call the remote identified by the passed feed ref
// retruns false if there is no such connection
func (n *node) GetEndpointFor(ref *ssb.FeedRef) (muxrpc.Endpoint, bool) {
	if ref == nil {
		return nil, false
	}
	n.remotesLock.Lock()
	defer n.remotesLock.Unlock()

	edp, has := n.remotes[ref.Ref()]
	return edp, has
}

func (n *node) addRemote(edp muxrpc.Endpoint) {
	n.remotesLock.Lock()
	defer n.remotesLock.Unlock()
	r, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		panic(err)
	}
	ref := r.Ref()
	if oldEdp, has := n.remotes[ref]; has {
		n.log.Log("remotes", "previous active", "ref", ref)
		c := client.FromEndpoint(oldEdp)
		_, err := c.Whoami()
		if err == nil {
			// old one still works
			return
		}
	}
	// replace with new
	n.remotes[r.Ref()] = edp
}

func (n *node) removeRemote(edp muxrpc.Endpoint) {
	n.remotesLock.Lock()
	defer n.remotesLock.Unlock()
	r, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		panic(err)
	}
	delete(n.remotes, r.Ref())
}

func (n *node) Serve(ctx context.Context, wrappers ...muxrpc.HandlerWrapper) error {
	if n.localDiscovTx != nil {
		// should also be called when ctx canceled?
		// or pass ctx to adv start?
		n.localDiscovTx.Start()

		defer n.localDiscovTx.Stop()
	}

	if n.localDiscovRx != nil {
		ch, done := n.localDiscovRx.Notify()
		defer done() // might trigger close of closed panic
		go func() {
			for a := range ch {
				if n.connTracker.Active(a) {
					//n.log.Log("event", "debug", "msg", "ignoring active", "addr", a.String())
					continue
				}
				err := n.Connect(ctx, a)
				if _, ok := errors.Cause(err).(secrethandshake.ErrProtocol); !ok {
					n.log.Log("event", "debug", "msg", "discovery dialback", "err", err)
				}

			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err := n.l.Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				// yikes way of handling this
				// but means this needs to be restarted anyway
				return nil
			}
			switch cause := errors.Cause(err).(type) {
			case secrethandshake.ErrProtocol:
				// ignore
			default:
				n.log.Log("msg", "node/Serve: failed to accept connection", "err", err,
					"cause", cause, "causeT", fmt.Sprintf("%T", cause))
			}

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
