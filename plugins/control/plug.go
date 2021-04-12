// SPDX-License-Identifier: MIT

package control

import (
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.mindeco.de/logging"
)

type connectPlug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, n ssb.Network, r ssb.Replicator) ssb.Plugin {
	return &connectPlug{h: New(i, n, r)}
}

func (p connectPlug) Name() string            { return "control" }
func (p connectPlug) Method() muxrpc.Method   { return muxrpc.Method{"ctrl"} }
func (p connectPlug) Handler() muxrpc.Handler { return p.h }
