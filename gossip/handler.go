package gossip

import (
	"context"
	"fmt"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/secretstream"
)

type Handler struct {
	Node sbot.Node
	Repo sbot.Repo
	Info logging.Interface

	Promisc bool

	lock          sync.RWMutex
	currentCaller sbot.Ref
}

func (g *Handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	g.lock.RLock()
	if g.currentCaller != nil {
		g.Info.Log("event", "busy error", "msg", "sorry - already busy", "currCaller", g.currentCaller.Ref())
		e.Terminate()
		return
	}
	g.lock.RUnlock()
	g.lock.Lock()
	defer func() {
		g.currentCaller = nil
		g.lock.Unlock()
	}()

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
	g.currentCaller = ref

	// fetch calling feed
	fref, ok := ref.(*sbot.FeedRef)
	if !ok {
		g.Info.Log("handleConnect", "notFeedRef", "r", shsID.String())
		return
	}

	if err := g.fetchFeed(ctx, *fref, e); err != nil {
		g.Info.Log("handleConnect", "fetchFeed remote failed", "r", fref.Ref(), "err", err)
		return
	}

	userFeeds := g.Repo.UserFeeds()
	mykp := g.Repo.KeyPair()
	hasOwn, err := multilog.Has(userFeeds, librarian.Addr(mykp.Id.ID))
	if err != nil {
		g.Info.Log("handleConnect", "multilog.Has(userFeeds,myID)", "err", err)
		return
	}

	if !hasOwn {
		g.Info.Log("handleConnect", "oops - dont have my own feed. requesting")
		if err := g.fetchFeed(ctx, mykp.Id, e); err != nil {
			g.Info.Log("handleConnect", "my fetchFeed failed", "r", mykp.Id.Ref(), "err", err)
			return
		}
	}

	/*
		if err != nil {
			g.Info.Log("handleConnect", "userFeeds failed", "err", err)
			return
		}
		n := len(kf)
		if n > 20 { // DOS / doublecall bug
			g.Info.Log("dbg", "shortening sync..", "n", n)
			n = 20
		}
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
				g.Info.Log("handleConnect", "fetchFeed failed", "err", err)
				return
			}
			n--
			if n == 0 {
				return
			}
		}
	*/
}

func (g *Handler) check(err error) {
	if err != nil {
		g.Info.Log("error", err)
		debug.PrintStack()
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
		g.check(req.Return(ctx, "connected"))

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
	return req.Stream.CloseWithError(errors.New("TODO:dos0day"))
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
	g.Info.Log("event", "doing gossip.connect", "remote", wrappedAddr.String())
	err = g.Node.Connect(ctx, wrappedAddr)
	return errors.Wrapf(err, "gossip.connect call: error connecting to %q", addr)
}
