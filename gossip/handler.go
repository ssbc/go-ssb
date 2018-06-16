package gossip

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/secretstream"
)

type Handler struct {
	Node sbot.Node
	Repo sbot.Repo
	Info logging.Interface

	// ugly hack
	running sync.Mutex
}

func (g *Handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	g.running.Lock()
	defer g.running.Unlock()

	srv := e.(muxrpc.Server)
	g.Info.Log("event", "onConnect", "handler", "gossip", "addr", srv.Remote())

	shsID, ok := netwrap.GetAddr(srv.Remote(), "shs-bs").(secretstream.Addr)
	if !ok {
		return
	}

	ref, err := sbot.ParseRef(shsID.String())
	if err != nil {
		g.Info.Log("handleConnect", "sbot.ParseRef", "err", err)
		return
	}

	fref, ok := ref.(*sbot.FeedRef)
	if !ok {
		g.Info.Log("handleConnect", "notFeedRef", "r", shsID.String())
		return
	}

	if err := g.fetchFeed(ctx, *fref, e); err != nil {
		g.Info.Log("handleConnect", "fetchFeed remote failed", "r", fref.Ref(), "err", err)
		return
	}

	mykp := g.Repo.KeyPair()
	if err := g.fetchFeed(ctx, mykp.Id, e); err != nil {
		g.Info.Log("handleConnect", "my fetchFeed failed", "r", mykp.Id.Ref(), "err", err)
		return
	}
	/*
		kf, err := g.Repo.KnownFeeds()
		if err != nil {
			g.Info.Log("handleConnect", "knownFeeds failed", "err", err)
			return
		}
		// lame init
		// kf["@f/6sQ6d2CMxRUhLpspgGIulDxDCwYD7DzFzPNr7u5AU=.ed25519"] = 0
		// kf["@EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=.ed25519"] = 0
		// kf["@YXkE3TikkY4GFMX3lzXUllRkNTbj5E+604AkaO1xbz8=.ed25519"] = 0

		for feed := range kf {
			ref, err := sbot.ParseRef(feed)
			if err != nil {
				g.Info.Log("handleConnect", "ParseRef failed", "err", err)
				return
			}

			fref, ok = ref.(*sbot.FeedRef)
			if !ok {
				g.Info.Log("handleConnect", "caset failed", "type", fmt.Sprintf("%T", ref))
				return
			}

			err = g.fetchFeed(ctx, *fref, e)
			if err != nil {
				g.Info.Log("handleConnect", "knownFeeds failed", "err", err)
				return
			}
		}
	*/
}

func (g *Handler) check(err error) {
	if err != nil {
		g.Info.Log("error", err)
	}
}

func (g *Handler) HandleCall(ctx context.Context, req *muxrpc.Request) {
	// g.Info.Log("event", "onCall", "handler", "gossip", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)

	var closed bool
	checkAndClose := func(err error) {
		g.check(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			g.check(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	defer func() {
		if !closed {
			g.check(errors.Wrapf(req.Stream.Close(), "gossip: error closing call: %s", req.Method))
		}
	}()

	switch req.Method.String() {
	case "createHistoryStream":
		if req.Type != "source" {
			checkAndClose(errors.Errorf("createHistoryStream: wrong tipe. %s", req.Type))
			return
		}
		if err := g.pourFeed(ctx, req); err != nil {
			checkAndClose(errors.Wrap(err, "createHistoryStream failed"))
			return
		}
		return

	case "gossip.ping":
		if err := g.ping(ctx, req); err != nil {
			checkAndClose(errors.Wrap(err, "gossip.ping failed."))
			return
		}

	case "gossip.connect":
		if len(req.Args) != 1 {
			// TODO: use secretstream
			g.Info.Log("error", "usage", "args", req.Args, "method", req.Method)
			checkAndClose(errors.New("usage: gossip.connect host:port:key"))
			return
		}
		destString, ok := req.Args[0].(string)
		if !ok {
			err := errors.Errorf("gossip.connect call: expected argument to be string, got %T", req.Args[0])
			checkAndClose(err)
			return
		}
		if err := g.connect(ctx, destString); err != nil {
			checkAndClose(errors.Wrap(err, "gossip.connect failed."))
			return
		}
	default:
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
	}
}

func (g *Handler) ping(ctx context.Context, req *muxrpc.Request) error {
	g.Info.Log("event", "ping", "args", fmt.Sprintf("%v", req.Args))
	for i := 0; i < 2; i++ {
		err := req.Stream.Pour(ctx, time.Now().Unix())
		if err != nil {
			return errors.Wrapf(err, "pour(%d) failed to pong", i)
		}
		time.Sleep(time.Second)
	}
	return req.Stream.Close()
}

func (g *Handler) connect(ctx context.Context, dest string) error {
	splitted := strings.Split(dest, ":")
	if n := len(splitted); n != 3 {
		return errors.Errorf("gossip.connect: bad request. expected 3 parts, got %d", n)
	}

	addr, err := net.ResolveTCPAddr("tcp", strings.Join(splitted[:2], ":"))
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: error resolving network address %q", splitted[:2])
	}

	ref, err := sbot.ParseRef(splitted[2])
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: failed to parse FeedRef %s", splitted[2])
	}

	remoteFeed, ok := ref.(*sbot.FeedRef)
	if !ok {
		return errors.Errorf("gossip.connect: expected FeedRef got %T", ref)
	}

	wrappedAddr := netwrap.WrapAddr(addr, secretstream.Addr{PubKey: remoteFeed.ID})
	err = g.Node.Connect(ctx, wrappedAddr)
	return errors.Wrapf(err, "gossip.connect call: error connecting to %q", addr)
}
