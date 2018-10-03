package sbot

import (
	"context"
	"io"
	"net"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
	"go.cryptoscope.co/sbot/graph"
	"go.cryptoscope.co/sbot/indexes"
	"go.cryptoscope.co/sbot/multilogs"
	"go.cryptoscope.co/sbot/plugins/blobs"
	"go.cryptoscope.co/sbot/plugins/gossip"
	"go.cryptoscope.co/sbot/plugins/whoami"
	"go.cryptoscope.co/sbot/repo"
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
			if err == nil {
				log.Log("event", "component terminated without error", "component", name)
				return
			}

			log.Log("event", "component terminated", "component", name, "error", err)
		}()
	}

	r := repo.New(s.repoPath)
	rootLog, err := repo.OpenLog(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open rootlog")
	}

	uf, _, serveUF, err := multilogs.OpenUserFeeds(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ...")
	}
	s.closers.addCloser(uf)
	goThenLog(ctx, rootLog, "userFeeds", serveUF)
	s.UserFeeds = uf

	gb, serveContacts, err := indexes.OpenContacts(kitlog.With(log, "index", "contacts"), r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ...")
	}
	s.closers.addCloser(gb)
	goThenLog(ctx, rootLog, "contacts", serveContacts)
	s.GraphBuilder = gb

	bs, err := repo.OpenBlobStore(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ...")
	}
	wm := blobstore.NewWantManager(kitlog.With(log, "module", "WantManager"), bs)

	s.KeyPair, err = repo.OpenKeyPair(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ...")
	}
	id := s.KeyPair.Id

	pmgr := sbot.NewPluginManager()

	// TODO get rid of this. either add error to pluginmgr.MakeHandler or take it away from the Options.
	errAdapter := func(mk func(net.Conn) muxrpc.Handler) func(net.Conn) (muxrpc.Handler, error) {
		return func(conn net.Conn) (muxrpc.Handler, error) {
			return mk(conn), nil
		}
	}

	opts := sbot.Options{
		ListenAddr:  s.listenAddr,
		KeyPair:     s.KeyPair,
		AppKey:      s.appKey[:],
		MakeHandler: graph.Authorize(kitlog.With(log, "module", "auth handler"), gb, id, 2, errAdapter(pmgr.MakeHandler)),
	}
	node, err := sbot.NewNode(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create node")
	}
	s.Node = node

	pmgr.Register(whoami.New(kitlog.With(log, "plugin", "whoami"), id))   // whoami
	pmgr.Register(blobs.New(kitlog.With(log, "plugin", "blobs"), bs, wm)) // blobs

	// gossip.*
	pmgr.Register(gossip.New(
		kitlog.With(log, "plugin", "gossip"),
		id, rootLog, uf, gb, node, false))

	// createHistoryStream
	pmgr.Register(gossip.NewHist(
		kitlog.With(log, "plugin", "gossip/hist"),
		id, rootLog, uf, gb, node))

	return s, nil
}
