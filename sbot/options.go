// SPDX-License-Identifier: MIT

// Package sbot ties together network, repo and plugins like graph and blobs into a large server that offers data-access APIs and background replication.
// It's name dates back to a time where ssb-server was still called scuttlebot, in short: sbot.
package sbot

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-kit/kit/metrics"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/netwrap"
	kitlog "go.mindeco.de/log"
	"go.mindeco.de/log/level"
	"go.mindeco.de/logging"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/internal/multicloser"
	"go.cryptoscope.co/ssb/internal/netwraputil"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
)

// MuxrpcEndpointWrapper can be used to wrap ever call a endpoint makes
type MuxrpcEndpointWrapper func(muxrpc.Endpoint) muxrpc.Endpoint

// Sbot is the database and replication server
type Sbot struct {
	info kitlog.Logger

	// TODO: this thing is way to big right now
	// because it's options and the resulting thing at once

	lateInit []Option

	rootCtx   context.Context
	Shutdown  context.CancelFunc
	closers   multicloser.MultiCloser
	idxDone   errgroup.Group
	idxInSync sync.WaitGroup

	closed   bool
	closedMu sync.Mutex
	closeErr error

	promisc  bool
	hopCount uint

	disableEBT                   bool
	disableLegacyLiveReplication bool

	// TODO: these should all be options that are applied on the network construction...
	Network            ssb.Network
	disableNetwork     bool
	appKey             []byte
	listenAddr         net.Addr
	dialer             netwrap.Dialer
	edpWrapper         MuxrpcEndpointWrapper
	networkConnTracker ssb.ConnTracker
	preSecureWrappers  []netwrap.ConnWrapper
	postSecureWrappers []netwrap.ConnWrapper

	public ssb.PluginManager
	master ssb.PluginManager

	authorizer ssb.Authorizer

	enableAdverts   bool
	enableDiscovery bool

	websocketAddr string

	repoPath string
	KeyPair  ssb.KeyPair

	Groups *private.Manager

	ReceiveLog multimsg.AlterableLog // the stream of messages as they arrived

	SeqResolver *repo.SequenceResolver

	PublishLog     ssb.Publisher
	signHMACsecret *[32]byte

	// hardcoded default indexes
	Users   *roaring.MultiLog // one sublog per feed
	Private *roaring.MultiLog // one sublog per keypair
	ByType  *roaring.MultiLog // one sublog per type: ... (special cases for private messages by suffix)
	Tangles *roaring.MultiLog // one sublog per root:%ref (actual root is in the get index)

	// plugin indexes
	mlogIndicies map[string]multilog.MultiLog
	simpleIndex  map[string]librarian.Index

	liveIndexUpdates bool
	indexStateMu     sync.Mutex
	indexStates      map[string]string

	ebtState *statematrix.StateMatrix

	GraphBuilder graph.Builder

	BlobStore   ssb.BlobStore
	WantManager ssb.WantManager

	// TODO: wrap better
	eventCounter metrics.Counter
	systemGauge  metrics.Gauge
	latency      metrics.Histogram

	ssb.Replicator
}

// Option is a functional option type definition to change sbot behaviour
type Option func(*Sbot) error

// WithBlobStore can be used to use a different storage backend for blobs.
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

// WithRepoPath changes where the replication database and blobs are stored.
func WithRepoPath(path string) Option {
	return func(s *Sbot) error {
		s.repoPath = path
		return nil
	}
}

// DisableNetworkNode disables all networking, in turn it only serves the database.
func DisableNetworkNode() Option {
	return func(s *Sbot) error {
		s.disableNetwork = true
		return nil
	}
}

// DisableEBT disables epidemic broadcast trees. It's a new implementation and might have some bugs.
func DisableEBT(yes bool) Option {
	return func(s *Sbot) error {
		s.disableEBT = yes
		return nil
	}
}

// DisableLegacyLiveReplication controls wether createHistoryStreams are created with live:true flag.
// This code is functional but might not scale to a lot of feeds. Therefore this flag can be used to force
// the old non-live polling mode.
func DisableLegacyLiveReplication(yes bool) Option {
	return func(s *Sbot) error {
		s.disableLegacyLiveReplication = yes
		return nil
	}
}

// WithListenAddr changes the muxrpc listener address. By default it listens to ':8008'.
func WithListenAddr(addr string) Option {
	return func(s *Sbot) error {
		var err error
		s.listenAddr, err = net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to parse tcp listen addr: %w", err)
		}
		return nil
	}
}

// WithDialer changes the function that is used to dial remote peers.
// This could be a sock5 connection builder to support tor proxying to hidden services.
func WithDialer(dial netwrap.Dialer) Option {
	return func(s *Sbot) error {
		s.dialer = dial
		return nil
	}
}

// WithNetworkConnTracker changes the connection tracker. See network.NewLastWinsTracker and network.NewAcceptAllTracker.
func WithNetworkConnTracker(ct ssb.ConnTracker) Option {
	return func(s *Sbot) error {
		s.networkConnTracker = ct
		return nil
	}
}

// WithUNIXSocket enables listening for muxrpc connections on a unix socket files ($repo/socket).
// This socket is not encrypted or authenticated since access to it is mediated by filesystem ownership.
func WithUNIXSocket() Option {
	return func(s *Sbot) error {
		// this races because sbot might not be done with init yet
		// TODO: refactor network peer code and make unixsock implement that (those will be inited late anyway)
		spoofWrapper := netwraputil.SpoofRemoteAddress(s.KeyPair.Id.PubKey())

		r := repo.New(s.repoPath)
		sockPath := r.GetPath("socket")

		// local clients (not using network package because we don't want conn limiting or advertising)
		c, err := net.Dial("unix", sockPath)
		if err == nil {
			c.Close()
			return fmt.Errorf("sbot: repo already in use, socket accepted connection")
		}
		os.Remove(sockPath)
		os.MkdirAll(filepath.Dir(sockPath), 0700)

		uxLis, err := net.Listen("unix", sockPath)
		if err != nil {
			return err
		}
		s.closers.AddCloser(uxLis)

		go func() {

			for {
				c, err := uxLis.Accept()
				if err != nil {
					if nerr, ok := err.(*net.OpError); ok {
						if nerr.Err.Error() == "use of closed network connection" {
							return
						}
					}

					err = fmt.Errorf("unix sock accept failed: %w", err)
					s.info.Log("warn", err)
					logging.CheckFatal(err)
					continue
				}

				wc, err := spoofWrapper(c)
				if err != nil {
					c.Close()
					continue
				}
				for _, w := range s.postSecureWrappers {
					var err error
					wc, err = w(wc)
					if err != nil {
						level.Warn(s.info).Log("err", err)
						c.Close()
						continue
					}
				}

				go func(conn net.Conn) {
					defer conn.Close()

					pkr := muxrpc.NewPacker(conn)

					h, err := s.master.MakeHandler(conn)
					if err != nil {
						err = fmt.Errorf("unix sock make handler: %w", err)
						s.info.Log("warn", err)
						logging.CheckFatal(err)
						return
					}

					edp := muxrpc.Handle(pkr, h,
						muxrpc.WithContext(s.rootCtx),
						muxrpc.WithLogger(kitlog.NewNopLogger()),
					)

					srv := edp.(muxrpc.Server)
					if err := srv.Serve(); err != nil {
						s.info.Log("conn", "serve exited", "err", err, "peer", conn.RemoteAddr())
					}
					edp.Terminate()

				}(wc)
			}
		}()
		return nil
	}
}

// WithAppKey changes the appkey (aka secret-handshake network cap).
// See https://ssbc.github.io/scuttlebutt-protocol-guide/#handshake for more.
func WithAppKey(k []byte) Option {
	return func(s *Sbot) error {
		if n := len(k); n != 32 {
			return fmt.Errorf("appKey: need 32 bytes got %d", n)
		}
		s.appKey = k
		return nil
	}
}

// WithNamedKeyPair changes from the default `secret` file, useful for testing.
func WithNamedKeyPair(name string) Option {
	return func(s *Sbot) error {
		r := repo.New(s.repoPath)
		var err error
		s.KeyPair, err = repo.LoadKeyPair(r, name)
		if err != nil {
			return fmt.Errorf("loading named key-pair %q failed: %w", name, err)
		}
		return nil
	}
}

// WithJSONKeyPair expectes a JSON-string as blob and calls ssb.ParseKeyPair on it.
// This is useful if you dont't want to place the keypair on the filesystem.
func WithJSONKeyPair(blob string) Option {
	return func(s *Sbot) error {
		var err error
		s.KeyPair, err = ssb.ParseKeyPair(strings.NewReader(blob))
		if err != nil {
			return fmt.Errorf("JSON KeyPair decode failed: %w", err)
		}
		return nil
	}
}

// WithKeyPair exepect a initialized ssb.KeyPair. Useful for testing.
func WithKeyPair(kp ssb.KeyPair) Option {
	return func(s *Sbot) error {
		s.KeyPair = kp
		return nil
	}
}

// WithInfo changes the info/warn/debug loging output.
func WithInfo(log kitlog.Logger) Option {
	return func(s *Sbot) error {
		s.info = log
		return nil
	}
}

// WithContext changes the context that is context.Background() by default.
// Handy to setup cancelation against a interup signal like ctrl+c.
// Canceling the context also shuts down indexing. If no context is passed sbot.Shutdown() can be used.
func WithContext(ctx context.Context) Option {
	return func(s *Sbot) error {
		s.rootCtx, s.Shutdown = ShutdownContext(ctx)
		return nil
	}
}

// TODO: remove all this network stuff and make them options on network

// WithPreSecureConnWrapper wrapps the connection after it is encrypted.
// Usefull for debugging and measuring traffic.
func WithPreSecureConnWrapper(cw netwrap.ConnWrapper) Option {
	return func(s *Sbot) error {
		s.preSecureWrappers = append(s.preSecureWrappers, cw)
		return nil
	}
}

// WithPostSecureConnWrapper wrapps the connection before it is encrypted.
// Usefull to insepct the muxrpc frames before they go into boxstream.
func WithPostSecureConnWrapper(cw netwrap.ConnWrapper) Option {
	return func(s *Sbot) error {
		s.postSecureWrappers = append(s.postSecureWrappers, cw)
		return nil
	}
}

// WithEventMetrics sets up latency and counter metrics
func WithEventMetrics(ctr metrics.Counter, lvls metrics.Gauge, lat metrics.Histogram) Option {
	return func(s *Sbot) error {
		s.eventCounter = ctr
		s.systemGauge = lvls
		s.latency = lat
		return nil
	}
}

// WithEndpointWrapper sets a MuxrpcEndpointWrapper for new connections.
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

// EnableAdvertismentDialing controls local peer discovery through listening for and connecting to UDP broadcasts
func EnableAdvertismentDialing(do bool) Option {
	return func(s *Sbot) error {
		s.enableDiscovery = do
		return nil
	}
}

// WithHMACSigning sets an HMAC signing key for messages.
// Useful for testing, see https://github.com/ssb-js/ssb-validate#state--validateappendstate-hmac_key-msg for more.
func WithHMACSigning(key []byte) Option {
	return func(s *Sbot) error {
		if n := len(key); n != 32 {
			return fmt.Errorf("WithHMACSigning: wrong key length (%d)", n)
		}
		var k [32]byte
		copy(k[:], key)
		s.signHMACsecret = &k
		return nil
	}
}

// WithWebsocketAddress changes the HTTP listener address, by default it's :8989.
func WithWebsocketAddress(addr string) Option {
	return func(s *Sbot) error {
		// TODO: listen here?
		s.websocketAddr = addr
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

// WithPublicAuthorizer configures who is considered "public" when accepting connections.
// By default, this is covered by the list of followed and blocked peers using the graph implementation.
func WithPublicAuthorizer(auth ssb.Authorizer) Option {
	return func(s *Sbot) error {
		if s.authorizer != nil {
			return fmt.Errorf("sbot: authorizer already configured")
		}
		s.authorizer = auth
		return nil
	}
}

// WithReplicator overwrites the default graph based decision maker, of which feeds to copy or block
func WithReplicator(r ssb.Replicator) Option {
	return func(s *Sbot) error {
		s.Replicator = r
		return nil
	}
}

// LateOption is a bit of a hack, it loads options after the _basic_ inititialisation is done (like repo location and keypair)
// this is mainly usefull for plugins that want to use a configured bot.
func LateOption(o Option) Option {
	return func(s *Sbot) error {
		s.lateInit = append(s.lateInit, o)
		return nil
	}
}

// New creates an sbot instance using the passed options to configure it.
func New(fopts ...Option) (*Sbot, error) {
	var s Sbot
	s.liveIndexUpdates = true

	s.public = ssb.NewPluginManager()
	s.master = ssb.NewPluginManager()

	s.mlogIndicies = make(map[string]multilog.MultiLog)
	s.simpleIndex = make(map[string]librarian.Index)
	s.indexStates = make(map[string]string)

	s.disableLegacyLiveReplication = true

	for i, opt := range fopts {
		err := opt(&s)
		if err != nil {
			return nil, fmt.Errorf("error applying option #%d: %w", i, err)
		}
	}

	if s.repoPath == "" {
		u, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("error getting info on current user: %w", err)
		}

		s.repoPath = filepath.Join(u.HomeDir, ".ssb-go")
	}

	if s.appKey == nil {
		ak, err := base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
		if err != nil {
			return nil, fmt.Errorf("failed to decode default appkey: %w", err)
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
		s.rootCtx, s.Shutdown = ShutdownContext(context.Background())
	}

	r := repo.New(s.repoPath)

	if len(s.KeyPair.Pair.Secret) == 0 {
		var err error
		s.KeyPair, err = repo.DefaultKeyPair(r)
		if err != nil {
			return nil, fmt.Errorf("sbot: failed to get keypair: %w", err)
		}
	}

	return initSbot(&s)
}
