// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package gossip

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/metrics"
	"github.com/ssbc/go-muxrpc/v2"
	"github.com/ssbc/go-ssb"
	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb/message"
	"github.com/ssbc/go-ssb/repo"
	"github.com/ssbc/margaret"
	"github.com/ssbc/margaret/multilog"
	"go.mindeco.de/log/level"
	"go.mindeco.de/logging"
)

// todo: make these proper functional options

type HMACSecret *[32]byte

type Promisc bool

type WithLive bool

type NumberOfConcurrentReplicationsPerPeer int
type NumberOfConcurrentReplications int

const defaultNumberOfConcurrentReplicationsPerPeer = 5
const defaultNumberOfConcurrentReplications = 10

// NewFetcher returns a muxrpc handler plugin which requests and verifies feeds, based on the passed replication lister.
func NewFetcher(
	ctx context.Context,
	log logging.Interface,
	r repo.Interface,
	id refs.FeedRef,
	rxlog margaret.Log,
	userFeeds multilog.MultiLog,
	fm *FeedManager,
	wantList ssb.ReplicationLister,
	vr *message.VerificationRouter,
	opts ...interface{},
) *plugin {
	h := &LegacyGossip{
		repo: r,

		ReceiveLog: rxlog,

		Id: id,

		UserFeeds:   userFeeds,
		feedManager: fm,
		WantList:    wantList,

		Info:    log,
		rootCtx: ctx,

		verifyRouter: vr,

		enableLiveStreaming: true,

		numberOfConcurrentReplicationsPerPeer: defaultNumberOfConcurrentReplicationsPerPeer,
		tokenPool:                             NewTokenPool(defaultNumberOfConcurrentReplications),
	}

	for i, o := range opts {
		switch v := o.(type) {
		case metrics.Gauge:
			h.sysGauge = v
		case metrics.Counter:
			h.sysCtr = v
		case HMACSecret:
			h.hmacSec = v
		case Promisc:
			h.promisc = bool(v)
		case WithLive:
			h.enableLiveStreaming = bool(v)
		case NumberOfConcurrentReplicationsPerPeer:
			h.numberOfConcurrentReplicationsPerPeer = int(v)
		case NumberOfConcurrentReplications:
			h.tokenPool = NewTokenPool(int(v))
		default:
			level.Warn(log).Log("event", "unhandled gossip option", "i", i, "type", fmt.Sprintf("%T", o))
		}
	}

	return &plugin{h}
}

// NewServer just handles the "supplying" side of gossip replication.
func NewServer(
	ctx context.Context,
	log logging.Interface,
	id refs.FeedRef,
	rxlog margaret.Log,
	userFeeds multilog.MultiLog,
	wantList ssb.ReplicationLister,
	fm *FeedManager,
	opts ...interface{},
) histPlugin {
	h := &LegacyGossip{
		Id: id,

		ReceiveLog:  rxlog,
		UserFeeds:   userFeeds,
		feedManager: fm,
		WantList:    wantList,

		Info:    log,
		rootCtx: ctx,

		numberOfConcurrentReplicationsPerPeer: defaultNumberOfConcurrentReplicationsPerPeer,
		tokenPool:                             NewTokenPool(defaultNumberOfConcurrentReplications),
	}

	for i, o := range opts {
		switch v := o.(type) {
		case metrics.Gauge:
			h.sysGauge = v
		case metrics.Counter:
			h.sysCtr = v
		case Promisc:
			h.promisc = bool(v)
		case HMACSecret:
			h.hmacSec = v
		case WithLive:
			// no consequence - the outgoing live code is fine
		case NumberOfConcurrentReplicationsPerPeer:
			h.numberOfConcurrentReplicationsPerPeer = int(v)
		case NumberOfConcurrentReplications:
			h.tokenPool = NewTokenPool(int(v))
		default:
			level.Warn(log).Log("event", "unhandled gossip option", "i", i, "type", fmt.Sprintf("%T", o))
		}
	}

	return histPlugin{h}
}

type plugin struct {
	*LegacyGossip
}

func (plugin) Name() string { return "gossip" }

func (plugin) Method() muxrpc.Method {
	return muxrpc.Method{"gossip"}
}

func (p plugin) Handler() muxrpc.Handler {
	return p.LegacyGossip
}

type histPlugin struct {
	*LegacyGossip
}

func (hp histPlugin) Name() string { return "createHistoryStream" }

func (histPlugin) Method() muxrpc.Method {
	return muxrpc.Method{"createHistoryStream"}
}

type IgnoreConnectHandler struct{ muxrpc.Handler }

func (IgnoreConnectHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

func (hp histPlugin) Handler() muxrpc.Handler {
	return IgnoreConnectHandler{hp.LegacyGossip}
}
