// SPDX-License-Identifier: MIT

package gossip

import (
	"context"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/message"
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

	sysGauge metrics.Gauge
	sysCtr   metrics.Counter

	feedManager *FeedManager
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

	info := log.With(g.Info, "remote", remoteRef.Ref()[1:5])

	if g.promisc {
		hasCallee, err := multilog.Has(g.UserFeeds, remoteRef.StoredAddr())
		if err != nil {
			info.Log("handleConnect", "multilog.Has(callee)", "err", err)
			return
		}

		if !hasCallee {
			info.Log("handleConnect", "oops - dont have feed of remote peer. requesting...")
			if err := g.fetchFeed(ctx, remoteRef, e); err != nil {
				info.Log("handleConnect", "fetchFeed callee failed", "err", err)
				return
			}
			info.Log("fetchFeed", "done callee")
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
		info.Log("handleConnect", "UserFeeds listing failed", "err", err)
		return
	}

	tGraph, err := g.GraphBuilder.Build()
	if err != nil {
		info.Log("handleConnect", "fetchFeed follows listing", "err", err)
		return
	}

	// TODO: port Blocked to FeedSet and make set operations
	var blockedAddr []librarian.Addr
	blocked := tGraph.BlockedList(g.Id)
	for _, ref := range ufaddrs {
		if _, isBlocked := blocked[ref]; isBlocked {
			level.Warn(info).Log("msg", "blocked feed still stored")
			blockedAddr = append(blockedAddr, ref)
		}
	}

	hops := g.GraphBuilder.Hops(g.Id, g.hopCount)
	if hops != nil {
		err := g.fetchAllMinus(ctx, e, hops, append(ufaddrs, blockedAddr...))
		if muxrpc.IsSinkClosed(err) || errors.Cause(err) == context.Canceled {
			return
		}
		if err != nil {
			level.Error(info).Log("fetching", "hops failed", "err", err)
		}
	}

	err = g.fetchAllLib(ctx, e, ufaddrs)
	if muxrpc.IsSinkClosed(err) || errors.Cause(err) == context.Canceled {
		return
	}
	if err != nil {
		level.Error(info).Log("fetching", "stored failed", "err", err)
	} else {
		info.Log("msg", "fetch done", "hops", hops.Count(), "stored", len(ufaddrs))
	}

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
			req.Stream.CloseWithError(err)
		} else {
			req.Stream.Close()
		}
	}

	switch req.Method.String() {

	case "createHistoryStream":
		//  https://ssbc.github.io/scuttlebutt-protocol-guide/#createHistoryStream
		args := req.Args()
		if req.Type != "source" {
			closeIfErr(errors.Errorf("wrong tipe. %s", req.Type))
			return
		}
		if len(args) < 1 {
			err := errors.New("ssb/message: not enough arguments, expecting feed id")
			closeIfErr(err)
			return
		}
		argMap, ok := args[0].(map[string]interface{})
		if !ok {
			err := errors.Errorf("ssb/message: not the right map - %T", args[0])
			closeIfErr(err)
			return
		}
		query, err := message.NewCreateHistArgsFromMap(argMap)
		if err != nil {
			closeIfErr(errors.Wrap(err, "bad request"))
			return
		}
		err = g.feedManager.CreateStreamHistory(ctx, req.Stream, query)
		if err != nil {
			req.Stream.CloseWithError(errors.Wrap(err, "createHistoryStream failed"))
			return
		}
		// don't close stream (feedManager will pass it on to live processing or close it itself)

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
