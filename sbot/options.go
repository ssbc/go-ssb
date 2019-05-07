package sbot

import (
	"context"
	"encoding/base64"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/network"
)

type MuxrpcEndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint

type Sbot struct {
	info kitlog.Logger

	repoPath   string
	dialer     netwrap.Dialer
	listenAddr net.Addr

	enableAdverts   bool
	enableDiscovery bool

	rootCtx        context.Context
	shutdownCancel context.CancelFunc
	closers        multiCloser

	appKey       []byte
	connWrappers []netwrap.ConnWrapper
	edpWrapper   MuxrpcEndpointWrapper

	RootLog      margaret.Log
	UserFeeds    multilog.MultiLog
	MessageTypes multilog.MultiLog
	PrivateLogs  multilog.MultiLog
	KeyPair      *ssb.KeyPair
	PublishLog   margaret.Log
	GraphBuilder graph.Builder
	Network      network.Interface
	// AboutStore   indexes.AboutStore
	BlobStore   ssb.BlobStore
	WantManager ssb.WantManager

	eventCounter *prometheus.Counter
	systemGauge  *prometheus.Gauge
	latency      *prometheus.Summary
}

type Option func(*Sbot) error

func WithRepoPath(path string) Option {
	return func(s *Sbot) error {
		s.repoPath = path
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

func WithAppKey(k []byte) Option {
	return func(s *Sbot) error {
		if n := len(k); n != 32 {
			return errors.Errorf("appKey: need 32 bytes got %d", n)
		}
		s.appKey = k
		return nil
	}
}

func WithJSONKeyPair(blob string) Option {
	return func(s *Sbot) error {
		var err error
		s.KeyPair, err = ssb.ParseKeyPair(strings.NewReader(blob))
		return errors.Wrap(err, "JSON KeyPair decode failed")
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

func WithConnWrapper(cw netwrap.ConnWrapper) Option {
	return func(s *Sbot) error {
		s.connWrappers = append(s.connWrappers, cw)
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

func New(fopts ...Option) (*Sbot, error) {
	var s Sbot
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

	return initSbot(&s)
}
