package control

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/network"
)

type connectPlug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, n network.Interface) ssb.Plugin {
	return &connectPlug{h: New(i, n)}
}

func (p connectPlug) Name() string {
	return "control"
}

func (p connectPlug) Method() muxrpc.Method {
	return muxrpc.Method{"ctrl"}
}

func (p connectPlug) Handler() muxrpc.Handler {
	return p.h
}
