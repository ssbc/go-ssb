package control

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
)

type plug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, n ssb.Node) ssb.Plugin {
	return &plug{h: New(i, n)}
}

func (p plug) Name() string {
	return "control"
}

func (p plug) Method() muxrpc.Method {
	return muxrpc.Method{"gossip"}
}

func (p plug) Handler() muxrpc.Handler {
	return p.h
}
