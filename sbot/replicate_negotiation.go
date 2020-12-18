package sbot

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"

	"go.cryptoscope.co/ssb/plugins/ebt"
	"go.cryptoscope.co/ssb/plugins/gossip"
)

type replicateNegotiator struct {
	logger log.Logger

	lg *gossip.LegacyGossip

	ebt *ebt.MUXRPCHandler
}

func (rn replicateNegotiator) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	// try ebt

	// the client calls ebt.replicate to the server
	if !muxrpc.IsServer(e) {
		// do nothing if we are the server

		rn.logger.Log("TODO", "we are server... start legacy after timeout?")
		// TODO: start legacy if remote doesn't start a ebt session after a timeout
		// if !rn.ebt.StartedSession(e.Remote()) {		}
		return
	}

	remote, err := ssb.GetFeedRefFromAddr(e.Remote())
	if err != nil {
		panic(err)
		return
	}

	level.Debug(rn.logger).Log("event", "triggering ebt.replicate", "r", remote.ShortRef())

	var opt = map[string]interface{}{"version": 3}

	// initiate ebt channel
	rx, tx, err := e.Duplex(ctx, muxrpc.TypeJSON, muxrpc.Method{"ebt", "replicate"}, opt)
	if err != nil {
		level.Debug(rn.logger).Log("event", "no ebt support", "err", err)

		// fallback to legacy
		rn.lg.StartLegacyFetching(ctx, e)
		return
	}

	rn.ebt.Loop(ctx, tx, rx, remote)
}

func (rn replicateNegotiator) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// noop - this handler only controls outgoing calls for replication
}

type negPlugin struct {
	replicateNegotiator
}

func (p negPlugin) Name() string {
	return "negotiate"
}

func (p negPlugin) Method() muxrpc.Method {
	return muxrpc.Method{"negotiate"}
}

func (p negPlugin) Handler() muxrpc.Handler {
	return p.replicateNegotiator
}
