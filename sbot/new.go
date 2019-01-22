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
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/plugins/blobs"
	"go.cryptoscope.co/ssb/plugins/control"
	"go.cryptoscope.co/ssb/plugins/gossip"
	"go.cryptoscope.co/ssb/plugins/whoami"
	"go.cryptoscope.co/ssb/repo"
)

type Interface interface {
	io.Closer
}

var _ Interface = (*Sbot)(nil)

func (s *Sbot) Close() error {
	return s.closers.Close()
}

func initSbot(s *Sbot) (*Sbot, error) {
	log := s.info
	ctx := s.rootCtx

	goThenLog := func(ctx context.Context, l margaret.Log, name string, f func(context.Context, margaret.Log) error) {
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

	gb, serveContacts, err := indexes.OpenContacts(kitlog.With(log, "index", "contacts"), r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open contacts idx")
	}
	s.closers.addCloser(gb)
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
	auth := gb.Authorizer(id, 4)

	publishLog, err := multilogs.OpenPublishLog(s.RootLog, s.UserFeeds, *s.KeyPair)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to create publish log")
	}

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
		id, rootLog, uf, gb, s.systemGauge, s.eventCounter,
		gossip.HopCount(3),
	))

	// incoming createHistoryStream handler
	hist := gossip.NewHist(
		kitlog.With(log, "plugin", "gossip/hist"),
		id, rootLog, uf, gb, s.systemGauge, s.eventCounter)
	pmgr.Register(hist)

	return s, nil
}
