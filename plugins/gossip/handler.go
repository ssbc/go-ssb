// SPDX-License-Identifier: MIT

package gossip

import (
	"context"
	"sync"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/metrics"
	"github.com/pkg/errors"
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

	hops := g.GraphBuilder.Hops(g.Id, g.hopCount)
	if hops != nil {
		err := g.fetchAll(ctx, e, hops)
		if muxrpc.IsSinkClosed(err) || errors.Cause(err) == context.Canceled {
			return
		}
		if err != nil {
			level.Error(info).Log("fetching", "hops failed", "err", err)
		}
	}
	level.Debug(info).Log("msg", "fetch done", "hops", hops.Count())
}

func (g *handler) HandleCall(
	ctx context.Context,
	req *muxrpc.Request,
	edp muxrpc.Endpoint,
) {
	if req.Type == "" {
		req.Type = "async"
	}

	hlog := log.With(g.Info, "method", req.Method.String())
	errLog := level.Error(hlog)
	dbgLog := level.Warn(hlog)

	closeIfErr := func(err error) {
		if err != nil {
			errLog.Log("event", "gossip call handler failed", "err", err)
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

		remote, err := ssb.GetFeedRefFromAddr(edp.Remote())
		if err != nil {
			closeIfErr(errors.Wrap(err, "bad remote"))
			return
		}

		// skip this check for self/master or in promisc mode (talk to everyone)
		if !(g.Id.Equal(remote) || g.promisc) {
			tg, err := g.GraphBuilder.Build()
			if err != nil {
				closeIfErr(errors.Wrap(err, "internal error"))
				return
			}

			if tg.Blocks(query.ID, remote) {
				dbgLog.Log("event", "feed blocked")
				req.Stream.Close()
				return
			}

			// see if there is a path from the wanted feed
			l, err := tg.MakeDijkstra(query.ID)
			if err != nil {
				errLog.Log("event", "graph dist lookup failed", "err", err)
				req.Stream.Close()
				return
			}

			// to the remote requesting it
			path, dist := l.Dist(remote)
			if len(path) < 1 || len(path) > 4 {
				errLog.Log("event", "requested feed doesnt know remote", "d", dist, "plen", len(path))
				req.Stream.Close()
				return
			}

			dbgLog.Log("event", "feeds in range", "d", dist, "plen", len(path))
			// now we know that at least someone they know, knows the remote
		} else {
			dbgLog.Log("event", "feed access granted")
		}

		err = g.feedManager.CreateStreamHistory(ctx, req.Stream, query)
		if err != nil {
			err = errors.Wrap(err, "createHistoryStream failed")
			closeIfErr(err)
			return
		}
		// don't close stream (feedManager will pass it on to live processing or close it itself)
		dbgLog.Log("event", "feed push done")

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
