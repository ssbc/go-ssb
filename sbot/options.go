// SPDX-License-Identifier: MIT

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
	"github.com/go-kit/kit/metrics"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/repo"
)

type MuxrpcEndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint

type Option func(*Sbot) error

func WithBlobStore(bs ssb.BlobStore) Option {
	return func(s *Sbot) error {
		s.BlobStore = bs
		return nil
	}
}

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

func WithEventMetrics(ctr metrics.Counter, lvls metrics.Gauge, lat metrics.Histogram) Option {
	return func(s *Sbot) error {
		s.eventCounter = ctr
		s.systemGauge = lvls
		s.latency = lat
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
