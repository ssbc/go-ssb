package ebt

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

type ebtPlug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, id *refs.FeedRef, rootLog margaret.Log, userFeeds multilog.MultiLog, wl ssb.ReplicationLister) ssb.Plugin {
	return &ebtPlug{h: New(i, id, rootLog, userFeeds, wl)}
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
