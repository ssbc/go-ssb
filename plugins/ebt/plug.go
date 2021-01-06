// SPDX-License-Identifier: MIT

package ebt

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins/gossip"
	refs "go.mindeco.de/ssb-refs"
)

type Plugin struct {
	*MUXRPCHandler
}

func NewPlug(
	i logging.Interface,
	id *refs.FeedRef,
	rootLog margaret.Log,
	uf multilog.MultiLog,
	wl ssb.ReplicationLister,
	fm *gossip.FeedManager,
	sm *statematrix.StateMatrix,
	v *message.VerifySink,
) *Plugin {

	return &Plugin{&MUXRPCHandler{
		info:      i,
		id:        id,
		rootLog:   rootLog,
		userFeeds: uf,
		wantList:  wl,

		livefeeds: fm,

		stateMatrix: sm,

		verify: v,

		currentMessages: make(map[string]refs.Message),
	},
	}
}

func (p Plugin) Name() string {
	return "ebt"
}

func (p Plugin) Method() muxrpc.Method {
	return muxrpc.Method{"ebt"}
}

func (p Plugin) Handler() muxrpc.Handler {
	return p.MUXRPCHandler
}
