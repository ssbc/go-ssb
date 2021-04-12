// SPDX-License-Identifier: MIT

// Package private suuplies an about to be deprecated way of accessing private messages.
// This supplies the private.read source from npm:ssb-private.
// In the  future relevant queries will have a private:true parameter to unbox readable messages automatically.
package private

import (
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/private"
	"go.mindeco.de/logging"
	refs "go.mindeco.de/ssb-refs"
)

type privatePlug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, author refs.FeedRef, mngr *private.Manager, publish ssb.Publisher, readIdx margaret.Log) ssb.Plugin {
	return &privatePlug{h: handler{
		author:  author,
		mngr:    mngr,
		publish: publish,
		read:    readIdx,
		info:    i,
	}}
}

func (p privatePlug) Name() string {
	return "private"
}

func (p privatePlug) Method() muxrpc.Method {
	return muxrpc.Method{"private"}
}

func (p privatePlug) Handler() muxrpc.Handler {
	return p.h
}
