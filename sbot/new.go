package sbot

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/cryptix/go/logging"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/plugins/blobs"
	"go.cryptoscope.co/ssb/plugins/control"
	"go.cryptoscope.co/ssb/plugins/gossip"
	privplug "go.cryptoscope.co/ssb/plugins/private"
	"go.cryptoscope.co/ssb/plugins/publish"
	"go.cryptoscope.co/ssb/plugins/rawread"
	"go.cryptoscope.co/ssb/plugins/whoami"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
)

type Interface interface {
	io.Closer
}

var _ Interface = (*Sbot)(nil)

func (s *Sbot) Close() error {
	// TODO: if already closed?
	if s.Network != nil {
		s.Network.GetConnTracker().CloseAll()
	}
	s.info.Log("event", "closing", "msg", "sbot close waiting for idxes")

	s.idxDone.Wait()
	// TODO: timeout?
	s.info.Log("event", "closing", "msg", "waited")

	if err := s.closers.Close(); err != nil {
		return err
	}
	s.info.Log("event", "closing", "msg", "closers closed")
	return nil
}

func initSbot(s *Sbot) (*Sbot, error) {
	log := s.info
	var ctx context.Context
	ctx, s.Shutdown = ctxutils.WithError(s.rootCtx, ssb.ErrShuttingDown)

	goThenLog := func(ctx context.Context, l margaret.Log, name string, f repo.ServeFunc) {
		s.idxDone.Add(1)
		go func(wg *sync.WaitGroup) {
			err := f(ctx, l, s.liveIndexUpdates)
			log.Log("event", "idx server exited", "idx", name, "error", err)
			if err != nil {
				err := s.Close()
				logging.CheckFatal(err)
				os.Exit(1)
				return
			}
			wg.Done()
		}(&s.idxDone)
	}

	r := repo.New(s.repoPath)
	rootLog, err := repo.OpenLog(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open rootlog")
	}
	s.closers.addCloser(rootLog.(io.Closer))
	s.RootLog = rootLog

	uf, _, serveUF, err := multilogs.OpenUserFeeds(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open user sublogs")
	}
	s.closers.addCloser(uf)
	goThenLog(ctx, rootLog, "userFeeds", serveUF)
	s.UserFeeds = uf

	mt, _, serveMT, err := multilogs.OpenMessageTypes(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open message type sublogs")
	}
	s.closers.addCloser(mt)
	goThenLog(ctx, rootLog, "msgTypes", serveMT)
	s.MessageTypes = mt

	/* new style graph builder
	contactLog, err := mt.Get(librarian.Addr("contact"))
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open message contact sublog")
	}
	s.GraphBuilder, err = graph.NewLogBuilder(s.info, mutil.Indirect(s.RootLog, contactLog))
	if err != nil {
		return nil, errors.Wrap(err, "sbot: NewLogBuilder failed")
	}
	*/

	gb, serveContacts, err := indexes.OpenContacts(kitlog.With(log, "module", "graph"), r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: OpenContacts failed")
	}
	goThenLog(ctx, rootLog, "contacts", serveContacts)
	s.GraphBuilder = gb

	bs, err := repo.OpenBlobStore(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open blob store")
	}
	s.BlobStore = bs
	wm := blobstore.NewWantManager(kitlog.With(log, "module", "WantManager"), bs, s.eventCounter, s.systemGauge)
	s.WantManager = wm

	if s.KeyPair == nil {
		s.KeyPair, err = repo.OpenKeyPair(r)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: failed to get keypair")
		}
	}
	id := s.KeyPair.Id
	auth := s.GraphBuilder.Authorizer(id, 2)

	if s.signHMACsecret != nil {
		publishLog, err := multilogs.OpenPublishLogWithHMAC(s.RootLog, s.UserFeeds, *s.KeyPair, s.signHMACsecret)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: failed to create publish log with hmac")
		}
		s.PublishLog = publishLog
	} else {
		publishLog, err := multilogs.OpenPublishLog(s.RootLog, s.UserFeeds, *s.KeyPair)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: failed to create publish log")
		}
		s.PublishLog = publishLog
	}

	pl, _, servePrivs, err := multilogs.OpenPrivateRead(kitlog.With(log, "module", "privLogs"), r, s.KeyPair)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to create privte read idx")
	}
	s.closers.addCloser(pl)
	goThenLog(ctx, rootLog, "privLogs", servePrivs)
	s.PrivateLogs = pl

	/* some randos
	ab, serveAbouts, err := indexes.OpenAbout(kitlog.With(log, "index", "contacts"), r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open about idx")
	}
	// s.closers.addCloser(ab)
	goThenLog(ctx, rootLog, "abouts", serveAbouts)
	s.AboutStore = ab

	// TODO: make these tests
	feeds := []string{
		s.KeyPair.Id.Ref(),
		"@uOReuhnb9+mPi5RnTbKMKRr3r87cK+aOg8lFXV/SBPU=.ed25519",
		"@EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=.ed25519",
		"@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519",
	}
	for _, feed := range feeds {
		fr, err := ssb.ParseFeedRef(feed)
		if err == nil {
			selfName, err := ab.GetName(fr)
			if err != nil {
				log.Log("event", "debug", "about", feed, "err", err)
				continue
			}
			goon.Dump(selfName)
		}
	}
	*/

	if s.disableNetwork {
		return s, nil
	}

	pmgr := ssb.NewPluginManager()
	ctrl := ssb.NewPluginManager()

	mkHandler := func(conn net.Conn) (muxrpc.Handler, error) {
		remote, err := ssb.GetFeedRefFromAddr(conn.RemoteAddr())
		if err != nil {
			return nil, errors.Wrap(err, "sbot: expected an address containing an shs-bs addr")
		}
		if bytes.Equal(id.ID, remote.ID) {
			return ctrl.MakeHandler(conn)
		}

		start := time.Now()
		err = auth.Authorize(remote)
		if s.latency != nil {
			s.latency.With("part", "graph_auth").Observe(time.Since(start).Seconds())
		}
		if err != nil {
			return nil, err
		}
		return pmgr.MakeHandler(conn)
	}

	opts := network.Options{
		Logger:           s.info,
		Dialer:           s.dialer,
		ListenAddr:       s.listenAddr,
		AdvertsSend:      s.enableAdverts,
		AdvertsConnectTo: s.enableDiscovery,
		KeyPair:          s.KeyPair,
		AppKey:           s.appKey[:],
		MakeHandler:      mkHandler,
		ConnWrappers:     s.connWrappers,

		EventCounter:    s.eventCounter,
		SystemGauge:     s.systemGauge,
		EndpointWrapper: s.edpWrapper,
		Latency:         s.latency,
	}

	node, err := network.New(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create network node")
	}
	s.Network = node
	s.closers.addCloser(s.Network)

	// TODO: should be gossip.connect but conflicts with our namespace assumption
	ctrl.Register(control.NewPlug(kitlog.With(log, "plugin", "ctrl"), node))

	ctrl.Register(publish.NewPlug(kitlog.With(log, "plugin", "publish"), s.PublishLog))
	userPrivs, err := pl.Get(librarian.Addr(s.KeyPair.Id.ID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to open user private index")
	}

	ctrl.Register(privplug.NewPlug(kitlog.With(log, "plugin", "private"), s.PublishLog, private.NewUnboxerLog(s.RootLog, userPrivs, s.KeyPair)))

	// whoami
	whoami := whoami.New(kitlog.With(log, "plugin", "whoami"), id)
	pmgr.Register(whoami)
	ctrl.Register(whoami)

	// blobs
	blobs := blobs.New(kitlog.With(log, "plugin", "blobs"), bs, wm)
	pmgr.Register(blobs)
	ctrl.Register(blobs) // TODO: does not need to open a createWants on this one?!

	// outgoing gossip behavior
	var histOpts = []interface{}{
		gossip.HopCount(3),
		s.systemGauge, s.eventCounter,
	}
	if s.signHMACsecret != nil {
		var k [32]byte
		copy(k[:], s.signHMACsecret)
		histOpts = append(histOpts, gossip.HMACSecret(&k))
	}
	pmgr.Register(gossip.New(
		kitlog.With(log, "plugin", "gossip"),
		id, rootLog, uf, s.GraphBuilder,
		histOpts...))

	// incoming createHistoryStream handler
	hist := gossip.NewHist(
		kitlog.With(log, "plugin", "gossip/hist"),
		id, rootLog, uf, s.GraphBuilder,
		histOpts...)
	pmgr.Register(hist)

	// raw log plugins
	ctrl.Register(rawread.NewRXLog(rootLog))      // createLogStream
	ctrl.Register(rawread.NewByType(rootLog, mt)) // messagesByType
	ctrl.Register(hist)                           // createHistoryStream

	return s, nil
}
