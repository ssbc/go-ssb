package sbot

import (
	"context"
	"encoding/base64"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/repo"
)

type MuxrpcEndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint

type Sbot struct {
	info kitlog.Logger

	// TODO: this thing is way to big right now
	// because it's options and the resulting thing at once

	lateInit []Option

	rootCtx  context.Context
	Shutdown context.CancelFunc
	closers  multiCloser
	idxDone  sync.WaitGroup

	promisc  bool
	hopCount uint

	// these should all be options that are applied on the network construction...
	Network        ssb.Network
	disableNetwork bool
	appKey         []byte
	listenAddr     net.Addr
	dialer         netwrap.Dialer
	edpWrapper     MuxrpcEndpointWrapper

	preSecureWrappers  []netwrap.ConnWrapper
	postSecureWrappers []netwrap.ConnWrapper

	public ssb.PluginManager
	master ssb.PluginManager

	enableAdverts   bool
	enableDiscovery bool

	repoPath         string
	KeyPair          *ssb.KeyPair
	RootLog          margaret.Log
	liveIndexUpdates bool

	PublishLog     ssb.Publisher
	signHMACsecret []byte

	mlogIndicies map[string]multilog.MultiLog
	simpleIndex  map[string]librarian.Index

	GraphBuilder graph.Builder

	BlobStore   ssb.BlobStore
	WantManager ssb.WantManager

	// TODO: wrap better
	eventCounter *prometheus.Counter
	systemGauge  *prometheus.Gauge
	latency      *prometheus.Summary
}

type Option func(*Sbot) error

// DisableLiveIndexMode makes the update processing halt once it reaches the end of the rootLog
// makes it easier to rebuild indicies.
func DisableLiveIndexMode() Option {
	return func(s *Sbot) error {
		s.liveIndexUpdates = false
		return nil
	}
}

func WithRepoPath(path string) Option {
	return func(s *Sbot) error {
		s.repoPath = path
		return nil
	}
}

func DisableNetworkNode() Option {
	return func(s *Sbot) error {
		s.disableNetwork = true
		return nil
	}
}

func WithListenAddr(addr string) Option {
	return func(s *Sbot) error {
		var err error
		s.listenAddr, err = net.ResolveTCPAddr("tcp", addr)
		return errors.Wrap(err, "failed to parse tcp listen addr")
	}
}

func WithDialer(dial netwrap.Dialer) Option {
	return func(s *Sbot) error {
		s.dialer = dial
		return nil
	}
}

func WithUNIXSocket() Option {
	return func(s *Sbot) error {
		r := repo.New(s.repoPath)
		sockPath := r.GetPath("socket")

		// local clients (not using network package because we don't want conn limiting or advertising)
		c, err := net.Dial("unix", sockPath)
		if err == nil {
			c.Close()
			return errors.Errorf("sbot: repo already in use, socket accepted connection")
		}
		os.Remove(sockPath)
		os.MkdirAll(filepath.Dir(sockPath), 0700)

		uxLis, err := net.Listen("unix", sockPath)
		if err != nil {
			return err
		}

		go func() {
			for {
				conn, err := uxLis.Accept()
				if err != nil {
					err = errors.Wrap(err, "unix sock accept failed")
					s.info.Log("warn", err)
					continue
				}

				go func() {
					pkr := muxrpc.NewPacker(conn)
					ctx, cancel := context.WithCancel(s.rootCtx)
					if cn, ok := pkr.(muxrpc.CloseNotifier); ok {
						closed := cn.Closed()
						go func() {
							<-closed
							cancel()
						}()
					}

					h, err := s.master.MakeHandler(conn)
					if err != nil {
						err = errors.Wrap(err, "unix sock make handler")
						s.info.Log("warn", err)
						cancel()
						return
					}

					// spoof remote as us
					sameAs := netwrap.WrapAddr(conn.RemoteAddr(), secretstream.Addr{PubKey: s.KeyPair.Id.ID})
					edp := muxrpc.HandleWithRemote(pkr, h, sameAs)

					srv := edp.(muxrpc.Server)
					if err := srv.Serve(ctx); err != nil {
						s.info.Log("conn", "serve exited", "err", err, "peer", conn.RemoteAddr())
					}
					cancel()
				}()
			}
		}()
		return nil
	}
}

func WithAppKey(k []byte) Option {
	return func(s *Sbot) error {
		if n := len(k); n != 32 {
			return errors.Errorf("appKey: need 32 bytes got %d", n)
		}
		s.appKey = k
		return nil
	}
}

func WithNamedKeyPair(name string) Option {
	return func(s *Sbot) error {
		r := repo.New(s.repoPath)
		var err error
		s.KeyPair, err = repo.LoadKeyPair(r, name)
		return errors.Wrapf(err, "loading named key-pair %q failed", name)
	}
}

func WithJSONKeyPair(blob string) Option {
	return func(s *Sbot) error {
		var err error
		s.KeyPair, err = ssb.ParseKeyPair(strings.NewReader(blob))
		return errors.Wrap(err, "JSON KeyPair decode failed")
	}
}

func WithKeyPair(kp *ssb.KeyPair) Option {
	return func(s *Sbot) error {
		s.KeyPair = kp
		return nil
	}
}

func WithInfo(log kitlog.Logger) Option {
	return func(s *Sbot) error {
		s.info = log
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(s *Sbot) error {
		s.rootCtx = ctx
		return nil
	}
}

// TODO: remove all this network stuff and make them options on network
func WithPreSecureConnWrapper(cw netwrap.ConnWrapper) Option {
	return func(s *Sbot) error {
		s.preSecureWrappers = append(s.preSecureWrappers, cw)
		return nil
	}
}

// TODO: remove all this network stuff and make them options on network
func WithPostSecureConnWrapper(cw netwrap.ConnWrapper) Option {
	return func(s *Sbot) error {
		s.postSecureWrappers = append(s.postSecureWrappers, cw)
		return nil
	}
}

func WithEventMetrics(ctr *prometheus.Counter, lvls *prometheus.Gauge, lat *prometheus.Summary) Option {
	return func(s *Sbot) error {
		s.eventCounter = ctr
		s.systemGauge = lvls
		s.latency = lat
		return nil
	}
}

func WithEndpointWrapper(mw MuxrpcEndpointWrapper) Option {
	return func(s *Sbot) error {
		s.edpWrapper = mw
		return nil
	}
}

// EnableAdvertismentBroadcasts controls local peer discovery through sending UDP broadcasts
func EnableAdvertismentBroadcasts(do bool) Option {
	return func(s *Sbot) error {
		s.enableAdverts = do
		return nil
	}
}

// EnableAdvertismentBroadcasts controls local peer discovery through listening for and connecting to UDP broadcasts
func EnableAdvertismentDialing(do bool) Option {
	return func(s *Sbot) error {
		s.enableDiscovery = do
		return nil
	}
}

func WithHMACSigning(key []byte) Option {
	return func(s *Sbot) error {
		if n := len(key); n != 32 {
			return errors.Errorf("WithHMACSigning: wrong key length (%d)", n)
		}
		s.signHMACsecret = key
		return nil
	}
}

// WithHops sets the number of friends (or bi-directionla follows) to walk between two peers
// controls fetch depth (whos feeds to fetch.
// 0: only my own follows
// 1: my friends follows
// 2: also their friends follows
// and how many hops a peer can be from self to for a connection to be accepted
func WithHops(h uint) Option {
	return func(s *Sbot) error {
		s.hopCount = h
		return nil
	}
}

// WithPromisc when enabled bypasses graph-distance lookups on connections and makes the gossip handler fetch the remotes feed
func WithPromisc(yes bool) Option {
	return func(s *Sbot) error {
		s.promisc = yes
		return nil
	}
}

func LateOption(o Option) Option {
	return func(s *Sbot) error {
		s.lateInit = append(s.lateInit, o)
		return nil
	}
}

func New(fopts ...Option) (*Sbot, error) {
	var s Sbot
	s.liveIndexUpdates = true

	s.public = ssb.NewPluginManager()
	s.master = ssb.NewPluginManager()

	s.mlogIndicies = make(map[string]multilog.MultiLog)
	s.simpleIndex = make(map[string]librarian.Index)

	for i, opt := range fopts {
		err := opt(&s)
		if err != nil {
			return nil, errors.Wrapf(err, "error applying option #%d", i)
		}
	}

	if s.repoPath == "" {
		u, err := user.Current()
		if err != nil {
			return nil, errors.Wrap(err, "error getting info on current user")
		}

		s.repoPath = filepath.Join(u.HomeDir, ".ssb-go")
	}

	if s.appKey == nil {
		ak, err := base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode default appkey")
		}
		s.appKey = ak
	}

	if s.dialer == nil {
		s.dialer = netwrap.Dial
	}

	if s.listenAddr == nil {
		s.listenAddr = &net.TCPAddr{Port: network.DefaultPort}
	}

	if s.info == nil {
		logger := kitlog.NewLogfmtLogger(kitlog.NewSyncWriter(os.Stdout))
		logger = kitlog.With(logger, "ts", kitlog.DefaultTimestampUTC, "caller", kitlog.DefaultCaller)
		s.info = logger
	}

	if s.rootCtx == nil {
		s.rootCtx = context.TODO()
	}

	r := repo.New(s.repoPath)

	if s.KeyPair == nil {
		var err error
		s.KeyPair, err = repo.DefaultKeyPair(r)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: failed to get keypair")
		}
	}

	return initSbot(&s)
}
