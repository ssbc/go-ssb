package ebt

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/numberedfeeds"
	"go.cryptoscope.co/ssb/internal/statematrix"
	refs "go.mindeco.de/ssb-refs"
)

type ebtPlug struct {
	h muxrpc.Handler
}

func NewPlug(
	i logging.Interface,
	id *refs.FeedRef,
	rootLog margaret.Log,
	uf multilog.MultiLog,
	wl ssb.ReplicationLister,
	nf *numberedfeeds.Index,
	sm *statematrix.StateMatrix,
) ssb.Plugin {

	return &ebtPlug{h: &handler{
		info:      i,
		id:        id,
		rootLog:   rootLog,
		userFeeds: uf,
		wantList:  wl,

		feedNumbers: nf,
		stateMatrix: sm,

		currentMessages: make(map[string]refs.Message),
	},
	}
}

func (p ebtPlug) Name() string {
	return "ebt"
}

func (p ebtPlug) Method() muxrpc.Method {
	return muxrpc.Method{"ebt"}
}

func (p ebtPlug) Handler() muxrpc.Handler {
	return p.h
}
