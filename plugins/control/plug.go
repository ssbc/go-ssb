package control

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
)

type plug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, n ssb.Node, publish margaret.Log) ssb.Plugin {
	return &plug{h: New(i, n, publish)}
}

func (p plug) Name() string {
	return "control"
}

func (p plug) Method() muxrpc.Method {
	return muxrpc.Method{"ctrl"}
}

func (p plug) Handler() muxrpc.Handler {
	return p.h
}
