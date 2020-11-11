// SPDX-License-Identifier: MIT

// Package partial is a helper module for ssb-browser-core, enabling to fetch subsets of feeds.
// See https://github.com/arj03/ssb-partial-replication for more.
package partial

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/muxmux"
	"go.cryptoscope.co/ssb/plugins/gossip"
)

type plugin struct {
	h muxrpc.Handler
}

const name = "partialReplication"

func (p plugin) Name() string {
	return name
}

func (p plugin) Method() muxrpc.Method {
	return muxrpc.Method{name}
}

func (p plugin) Handler() muxrpc.Handler {
	return p.h
}

func New(log logging.Interface,
	fm *gossip.FeedManager,
	feeds, bytype, roots *roaring.MultiLog,
	rxlog margaret.Log,
	get ssb.Getter,
) ssb.Plugin {
	rootHdlr := muxmux.New(log)

	rootHdlr.RegisterAsync(muxrpc.Method{name, "getTangle"}, getTangleHandler{
		roots: roots,
		get:   get,
		rxlog: rxlog,
	})

	rootHdlr.RegisterSource(muxrpc.Method{name, "getFeed"}, getFeedHandler{
		fm: fm,
	})

	rootHdlr.RegisterSource(muxrpc.Method{name, "getFeedReverse"}, getFeedReverseHandler{
		fm: fm,
	})

	rootHdlr.RegisterSource(muxrpc.Method{name, "getMessagesOfType"}, getMessagesOfTypeHandler{
		rxlog: rxlog,

		feeds:  feeds,
		bytype: bytype,
	})

	return plugin{
		h: &rootHdlr,
	}
}
