package sbot

import (
	"context"
	"encoding/base64"
	"go.cryptoscope.co/margaret"
	"net"
	"os"
	"os/user"
	"path/filepath"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/graph"
)

type Sbot struct {
	repoPath     string
	listenAddr   net.Addr
	info         kitlog.Logger
	rootCtx      context.Context
	appKey       []byte
	closers      multiCloser
	connWrappers []netwrap.ConnWrapper

	RootLog      margaret.Log
	UserFeeds    multilog.MultiLog
	KeyPair      *sbot.KeyPair
	GraphBuilder graph.Builder
	Node         sbot.Node
	BlobStore    sbot.BlobStore
	WantManager  sbot.WantManager
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
func WithAppKey(k []byte) Option {
	return func(s *Sbot) error {
		if n := len(k); n != 32 {
			return errors.Errorf("appKey: need 32 bytes got %d", n)
		}
		s.appKey = k
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

func WithConnWrapper(cw netwrap.ConnWrapper) Option {
	return func(s *Sbot) error {
		s.connWrappers = append(s.connWrappers, cw)
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

	if s.listenAddr == nil {
		s.listenAddr = &net.TCPAddr{Port: 8008}
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
