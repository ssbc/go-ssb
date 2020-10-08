// SPDX-License-Identifier: MIT

package private

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/private/box"
)

type privatePlug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, publish ssb.Publisher, readIdx margaret.Log) ssb.Plugin {
	return &privatePlug{h: handler{
		boxer:   box.NewBoxer(nil),
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
