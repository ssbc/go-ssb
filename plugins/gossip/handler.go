package gossip

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
)

type handler struct {
	Id           *ssb.FeedRef
	RootLog      margaret.Log
	UserFeeds    multilog.MultiLog
	GraphBuilder graph.Builder
	Info         logging.Interface

	hopCount int
	promisc  bool // ask for remote feed even if it's not on owns fetch list

	activeLock  sync.Mutex
	activeFetch sync.Map

	hanlderDone func()

	sysGauge *prometheus.Gauge
	sysCtr   *prometheus.Counter
}

func (g *handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	defer func() {
		// TODO: just used for testing...
		// maybe make an interface wrapper instead
		g.hanlderDone()
	}()

	hasOwn, err := multilog.Has(g.UserFeeds, librarian.Addr(g.Id.ID))
	if err != nil {
		g.Info.Log("handleConnect", "multilog.Has(g.UserFeeds,myID)", "err", err)
		return
	}

	if !hasOwn {
		g.Info.Log("handleConnect", "oops - dont have my own feed. requesting")
		if err := g.fetchFeed(ctx, g.Id, e); err != nil {
			g.Info.Log("handleConnect", "my fetchFeed failed", "r", g.Id.Ref(), "err", err)
			return
		}
		g.Info.Log("fetchFeed", "done self")
	}

	if g.promisc {
		remote := e.(muxrpc.Server).Remote()
		remoteAddr, ok := netwrap.GetAddr(remote, "shs-bs").(secretstream.Addr)
		if !ok {
			return
		}
		remoteRef := &ssb.FeedRef{
			Algo: "ed25519",
			ID:   remoteAddr.PubKey,
		}

		hasCallee, err := multilog.Has(g.UserFeeds, librarian.Addr(remoteRef.ID))
		if err != nil {
			g.Info.Log("handleConnect", "multilog.Has(callee)", "ref", remoteRef.Ref(), "err", err)
			return
		}

		if !hasCallee {
			g.Info.Log("handleConnect", "oops - dont have calling feed. requesting")
			if err := g.fetchFeed(ctx, remoteRef, e); err != nil {
				g.Info.Log("handleConnect", "fetchFeed callee failed", "ref", remoteRef.Ref(), "err", err)
				return
			}
			g.Info.Log("fetchFeed", "done callee", "ref", remoteRef.Ref())
		}
	}

	ufaddrs, err := g.UserFeeds.List()
	if err != nil {
		g.Info.Log("handleConnect", "UserFeeds listing failed", "err", err)
		return
	}

	tGraph, err := g.GraphBuilder.Build()
	if err != nil {
		g.Info.Log("handleConnect", "fetchFeed follows listing", "err", err)
		return
	}

	var blockedAddr []librarian.Addr
	blocked := tGraph.BlockedList(g.Id)
	for _, ref := range ufaddrs {
		var k [32]byte
		copy(k[:], []byte(ref))
		if _, isBlocked := blocked[k]; isBlocked {
			blockedAddr = append(blockedAddr, ref)
		}
	}

	err = g.fetchAllLib(ctx, e, ufaddrs)
	if muxrpc.IsSinkClosed(err) {
		return
	}

	// follows, err := g.GraphBuilder.Follows(g.Id)
	// if err != nil {
	// 	g.Info.Log("msg", "fetchFeed follows listing", "err", err)
	// 	return
	// }
	// g.fetchAllMinus(ctx, e, follows, ufaddrs)

	hops := g.GraphBuilder.Hops(g.Id, g.hopCount)
	g.fetchAllMinus(ctx, e, hops, append(ufaddrs, blockedAddr...))
	g.Info.Log("msg", "fetchHops done", "n", hops.Count())

}

func (h *handler) fetchAllLib(ctx context.Context, e muxrpc.Endpoint, lst []librarian.Addr) error {
	var refs = graph.NewFeedSet(len(lst))
	for _, addr := range lst {
		refs.AddAddr(addr)
	}
	return h.fetchAll(ctx, e, refs)
}

func (h *handler) fetchAllMinus(ctx context.Context, e muxrpc.Endpoint, fs graph.FeedSet, got []librarian.Addr) error {
	lst := fs.List()
	var refs = graph.NewFeedSet(len(lst))
	for _, ref := range lst {
		if !isIn(got, ref) {
			refs.AddRef(ref)
		}
	}
	return h.fetchAll(ctx, e, refs)
}

func (h *handler) fetchAll(ctx context.Context, e muxrpc.Endpoint, fs graph.FeedSet) error {
	// var wg sync.WaitGroup
	lst := fs.List()
	// wg.Add(len(lst))
	for _, r := range lst {

		// go func(ref *ssb.FeedRef, wg *sync.WaitGroup) {
		// 	defer wg.Done()
		err := h.fetchFeed(ctx, r, e)
		if muxrpc.IsSinkClosed(err) {
			return err
		} else if err != nil {
			// assuming forked feed for instance
			h.Info.Log("msg", "fetchFeed stored failed", "err", err)
		}
		// }(r, &wg)
	}
	return nil
	// wg.Wait()
}
func isIn(list []librarian.Addr, a *ssb.FeedRef) bool {
	for _, el := range list {
		if bytes.Compare([]byte(el), a.ID) == 0 {
			return true
		}
	}
	return false
}

func (g *handler) check(err error) {
	if err != nil {
		g.Info.Log("error", err)
	}
}

func (g *handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// g.Info.Log("event", "onCall", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	if req.Type == "" {
		req.Type = "async"
	}

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

	default:
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
	}
}

func (g *handler) ping(ctx context.Context, req *muxrpc.Request) error {
	err := req.Stream.Pour(ctx, time.Now().UnixNano()/1000000)
	if err != nil {
		return errors.Wrapf(err, "pour failed to pong")
	}
	// just leave it open..
	return nil
}
