package sbot

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"time"

	"github.com/cryptix/go/logging"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/plugins/blobs"
	"go.cryptoscope.co/ssb/plugins/control"
	"go.cryptoscope.co/ssb/plugins/gossip"
	"go.cryptoscope.co/ssb/plugins/rawread"
	"go.cryptoscope.co/ssb/plugins/whoami"
	"go.cryptoscope.co/ssb/repo"
)

type Interface interface {
	io.Closer
}

var _ Interface = (*Sbot)(nil)

func (s *Sbot) Close() error {
	s.shutdownCancel()
	// TODO: just a sleep-kludge
	// would be better to have a busy-loop or channel thing to see once everything is closed
	time.Sleep(time.Second * 1)
	return s.closers.Close()
}

type margaretServe func(context.Context, margaret.Log) error

func initSbot(s *Sbot) (*Sbot, error) {
	log := s.info
	var ctx context.Context
	ctx, s.shutdownCancel = ctxutils.WithError(s.rootCtx, ssb.ErrShuttingDown)

	goThenLog := func(ctx context.Context, l margaret.Log, name string, f margaretServe) {
		go func() {
			err := f(ctx, l)
			if err != nil {
				log.Log("event", "component terminated", "component", name, "error", err)
				err := s.Close()
				logging.CheckFatal(err)
				os.Exit(1)
				return
			}
		}()
	}

	r := repo.New(s.repoPath)
	rootLog, err := repo.OpenLog(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open rootlog")
	}
	s.RootLog = rootLog

	uf, _, serveUF, err := multilogs.OpenUserFeeds(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open user sublogs")
	}
	s.closers.addCloser(uf)
	goThenLog(ctx, rootLog, "userFeeds", serveUF)
	s.UserFeeds = uf

	/* new style graph builder
	mt, _, serveMT, err := multilogs.OpenMessageTypes(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open message type sublogs")
	}
	s.closers.addCloser(mt)
	goThenLog(ctx, rootLog, "msgTypes", serveMT)
	s.MessageTypes = mt

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

	publishLog, err := multilogs.OpenPublishLog(s.RootLog, s.UserFeeds, *s.KeyPair)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to create publish log")
	}
	s.PublishLog = publishLog

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

	opts := ssb.Options{
		Logger:       s.info,
		Dialer:       s.dialer,
		ListenAddr:   s.listenAddr,
		KeyPair:      s.KeyPair,
		AppKey:       s.appKey[:],
		MakeHandler:  mkHandler,
		ConnWrappers: s.connWrappers,

		EventCounter:    s.eventCounter,
		SystemGauge:     s.systemGauge,
		EndpointWrapper: s.edpWrapper,
		Latency:         s.latency,
	}

	node, err := ssb.NewNode(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create node")
	}
	s.Node = node
	s.closers.addCloser(s.Node)

	ctrl.Register(control.NewPlug(kitlog.With(log, "plugin", "ctrl"), node, publishLog))

	// whoami
	whoami := whoami.New(kitlog.With(log, "plugin", "whoami"), id)
	pmgr.Register(whoami)
	ctrl.Register(whoami)

	// blobs
	blobs := blobs.New(kitlog.With(log, "plugin", "blobs"), bs, wm)
	pmgr.Register(blobs)
	ctrl.Register(blobs) // TODO: does not need to open a createWants on this one?!

	// outgoing gossip behavior
	pmgr.Register(gossip.New(
		kitlog.With(log, "plugin", "gossip"),
		id, rootLog, uf, s.GraphBuilder, s.systemGauge, s.eventCounter,
		gossip.HopCount(3),
	))

	// incoming createHistoryStream handler
	hist := gossip.NewHist(
		kitlog.With(log, "plugin", "gossip/hist"),
		id, rootLog, uf, s.GraphBuilder, s.systemGauge, s.eventCounter)
	pmgr.Register(hist)

	// raw log plugins
	ctrl.Register(rawread.NewRXLog(rootLog)) // createLogStream
	// ctrl.Register(rawread.NewByType(rootLog, mt)) // messagesByType
	ctrl.Register(hist) // createHistoryStream

	return s, nil
}
