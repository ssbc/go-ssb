// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ebt

import (
	"sync"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/muxrpc/v2/typemux"
	"go.mindeco.de/log"

	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins/gossip"
	refs "go.mindeco.de/ssb-refs"
)

type Plugin struct {
	*Replicate

	muxer *typemux.HandlerMux
}

func NewPlug(
	i log.Logger,
	self refs.FeedRef,
	rxLog margaret.Log,
	uf multilog.MultiLog,
	gb *graph.BadgerBuilder,
	fm *gossip.FeedManager,
	sm *statematrix.StateMatrix,
	v *message.VerificationRouter,
) *Plugin {

	r := &Replicate{
		info:       i,
		self:       self,
		receiveLog: rxLog,
		userFeeds:  uf,

		graph:         gb,
		idx2authCache: make(idx2authorCacheMap),

		livefeeds: fm,

		stateMatrix: sm,

		verify: v,

		Sessions: Sessions{
			mu:   new(sync.Mutex),
			open: make(map[string]*session),

			waitingFor: make(map[string]chan<- struct{}),
		},
	}

	mux := typemux.New(i)

	mux.RegisterDuplex(muxrpc.Method{"ebt", "replicate"}, typemux.DuplexFunc(r.HandleLegacy))
	mux.RegisterDuplex(muxrpc.Method{"ebt", "replicateFormat"}, typemux.DuplexFunc(r.HandleFormat))
	mux.RegisterAsync(muxrpc.Method{"ebt", "clock"}, typemux.AsyncFunc(r.Clock))

	plug := &Plugin{
		Replicate: r,

		muxer: &mux,
	}

	return plug
}

// muxrpc plugin
func (p Plugin) Name() string            { return "ebt" }
func (p Plugin) Method() muxrpc.Method   { return muxrpc.Method{"ebt"} }
func (p Plugin) Handler() muxrpc.Handler { return p.muxer }
