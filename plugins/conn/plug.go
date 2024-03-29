// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package conn

import (
	"github.com/ssbc/go-muxrpc/v2"
	"github.com/ssbc/go-ssb"
	"go.mindeco.de/logging"
)

type connectPlug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, n ssb.Network, r ssb.Replicator) ssb.Plugin {
	return &connectPlug{h: New(i, n, r)}
}

func (p connectPlug) Name() string            { return "conn" }
func (p connectPlug) Method() muxrpc.Method   { return muxrpc.Method{"conn"} }
func (p connectPlug) Handler() muxrpc.Handler { return p.h }
