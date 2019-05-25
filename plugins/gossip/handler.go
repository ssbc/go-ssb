package gossip

import (
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
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
)

type handler struct {
	Id           *ssb.FeedRef
	RootLog      margaret.Log
	UserFeeds    multilog.MultiLog
	GraphBuilder graph.Builder
	Info         logging.Interface

	hmacSec  HMACSecret
	hopCount int
	promisc  bool // ask for remote feed even if it's not on owns fetch list

	activeLock  sync.Mutex
	activeFetch sync.Map

	sysGauge *prometheus.Gauge
	sysCtr   *prometheus.Counter
}

func (g *handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	remote := e.Remote()
	remoteRef, err := ssb.GetFeedRefFromAddr(remote)
	if err != nil {
		return
	}

	if remoteRef.Equal(g.Id) {
		return
	}

	if g.promisc {
		hasCallee, err := multilog.Has(g.UserFeeds, remoteRef.StoredAddr())
		if err != nil {
			g.Info.Log("handleConnect", "multilog.Has(callee)", "ref", remoteRef.Ref(), "err", err)
			return
		}

		if !hasCallee {
			g.Info.Log("handleConnect", "oops - dont have feed of remote peer. requesting...")
			if err := g.fetchFeed(ctx, remoteRef, e); err != nil {
				g.Info.Log("handleConnect", "fetchFeed callee failed", "ref", remoteRef.Ref(), "err", err)
				return
			}
			g.Info.Log("fetchFeed", "done callee", "ref", remoteRef.Ref())
		}
	}

	// TODO: ctx to build and list?!
	// or pass rootCtx to their constructor but than we can't cancel sessions
	select {
	case <-ctx.Done():
		return
	default:
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

	// TODO: port Blocked to FeedSet and make set operations
	var blockedAddr []librarian.Addr
	blocked := tGraph.BlockedList(g.Id)
	for _, ref := range ufaddrs {
		if _, isBlocked := blocked[ref]; isBlocked {
			blockedAddr = append(blockedAddr, ref)
		}
	}

	hops := g.GraphBuilder.Hops(g.Id, g.hopCount)
	if hops != nil {
		err := g.fetchAllMinus(ctx, e, hops, append(ufaddrs, blockedAddr...))
		if muxrpc.IsSinkClosed(err) || errors.Cause(err) == context.Canceled {
			return
		}
	}

	err = g.fetchAllLib(ctx, e, ufaddrs)
	if muxrpc.IsSinkClosed(err) || errors.Cause(err) == context.Canceled {
		return
	}

	g.Info.Log("msg", "fetchHops done", "hops", hops.Count(), "stored", len(ufaddrs))
}

func (g *handler) check(err error) {
	if err != nil {
		g.Info.Log("error", err)
	}
}

func (g *handler) HandleCall(
	ctx context.Context,
	req *muxrpc.Request,
	edp muxrpc.Endpoint,
) {
	if req.Type == "" {
		req.Type = "async"
	}

	closeIfErr := func(err error) {
		g.check(err)
		if err != nil {
			closeErr := req.Stream.CloseWithError(err)
			g.check(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	switch req.Method.String() {

	case "createHistoryStream":
		if req.Type != "source" {
			closeIfErr(errors.Errorf("createHistoryStream: wrong tipe. %s", req.Type))
			return
		}
		if err := g.pourFeed(ctx, req); err != nil {
			closeIfErr(errors.Wrap(err, "createHistoryStream failed"))
			return
		}

	case "gossip.ping":
		err := req.Stream.Pour(ctx, time.Now().UnixNano()/1000000)
		if err != nil {
			closeIfErr(errors.Wrapf(err, "pour failed to pong"))
			return
		}
		// just leave this stream open.
		// some versions of ssb-gossip don't like if the stream is closed without an error

	default:
		closeIfErr(errors.Errorf("unknown command: %s", req.Method))
	}
}
