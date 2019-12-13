package sbot

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.mindeco.de/logging"

	"go.cryptoscope.co/ssb/internal/netwraputil"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/repo"
)

func WithNetwork(opt network.Options) Option {
	return func(s *Sbot) error {
		if opt.MakeHandler == nil {
			opt.MakeHandler = s.mkHandler
		}

		if opt.KeyPair == nil {
			opt.KeyPair = s.KeyPair
		}

		if opt.AppKey == nil {
			opt.AppKey = s.appKey[:]
		}

		nw, err := network.New(opt)
		if err != nil {
			return errors.Wrap(err, "sbot: network creation failed")
		}
		s.networks = append(s.networks, nw)
		return nil
	}
}

func WithDefaultTCPNetwork() Option {
	return func(s *Sbot) error {

		o := network.Options{
			Logger: s.info,
			// Dialer:              s.dialer,
			ListenAddr: &net.TCPAddr{Port: network.DefaultPort},
			// AdvertsSend:      s.enableAdverts,
			// AdvertsConnectTo: s.enableDiscovery,
			KeyPair:     s.KeyPair,
			AppKey:      s.appKey[:],
			MakeHandler: s.mkHandler,
			// ConnTracker:      nil,
			// BefreCryptoWrappers: s.preSecureWrappers,
			// AfterSecureWrappers: s.postSecureWrappers,

			EventCounter: s.eventCounter,
			SystemGauge:  s.systemGauge,
			// EndpointWrapper: s.edpWrapper,
			Latency: s.latency,
		}
		nw, err := network.New(o)
		if err != nil {
			return errors.Wrap(err, "sbot: network creation failed")
		}
		s.networks = append(s.networks, nw)
		return nil
	}
}

func (s *Sbot) ServeAll(ctx context.Context) {
	for _, n := range s.networks {
		s.muxservGroup.Go(servNetwork(n, ctx))
	}
}

func servNetwork(nw ssb.Network, ctx context.Context) func() error {
	return func() error {
		return nw.Serve(ctx)
	}
}

/* TODO: port these to network options

func WithEndpointWrapper(mw MuxrpcEndpointWrapper) Option {
	return func(s *Sbot) error {
		s.edpWrapper = mw
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

func WithNetworkConnTracker(ct ssb.ConnTracker) Option {
	return func(s *Sbot) error {
		s.networkConnTracker = ct
		return nil
	}
}

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


*/

func WithUNIXSocket() Option {
	return func(s *Sbot) error {
		// this races because sbot might not be done with init yet
		// TODO: refactor network peer code and make unixsock implement that (those will be inited late anyway)
		if s.KeyPair == nil {
			return errors.Errorf("sbot/unixsock: keypair is nil. please use unixSocket with LateOption")
		}
		spoofWrapper := netwraputil.SpoofRemoteAddress(s.KeyPair.Id.ID)

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
		s.closers.addCloser(uxLis)

		go func() {

			for {
				c, err := uxLis.Accept()
				if err != nil {
					if nerr, ok := err.(*net.OpError); ok {
						if nerr.Err.Error() == "use of closed network connection" {
							return
						}
					}

					err = errors.Wrap(err, "unix sock accept failed")
					s.info.Log("warn", err)
					logging.CheckFatal(err)
					continue
				}

				wc, err := spoofWrapper(c)
				if err != nil {
					c.Close()
					continue
				}
				go func(conn net.Conn) {
					defer conn.Close()

					pkr := muxrpc.NewPacker(conn)

					h, err := s.master.MakeHandler(conn)
					if err != nil {
						err = errors.Wrap(err, "unix sock make handler")
						s.info.Log("warn", err)
						logging.CheckFatal(err)
						return
					}

					edp := muxrpc.HandleWithLogger(pkr, h, s.info)

					ctx, cancel := context.WithCancel(s.rootCtx)
					srv := edp.(muxrpc.Server)
					if err := srv.Serve(ctx); err != nil {
						s.info.Log("conn", "serve exited", "err", err, "peer", conn.RemoteAddr())
					}
					edp.Terminate()
					cancel()
				}(wc)
			}
		}()
		return nil
	}
}
