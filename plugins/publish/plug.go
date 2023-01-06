// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// Package publish is just a muxrpc wrapper around sbot.PublishLog.Publish.
package publish

import (
	"sync"

	"github.com/ssbc/go-muxrpc/v2"
	"github.com/ssbc/go-muxrpc/v2/typemux"
	"github.com/ssbc/go-ssb"
	"github.com/ssbc/go-ssb/private"
	"github.com/ssbc/margaret"
	"go.mindeco.de/logging"
)

type publishPlug struct{ h muxrpc.Handler }

func NewPlug(
	i logging.Interface,
	publish ssb.Publisher,
	boxer *private.Manager,
	authorLog margaret.Log,
) ssb.Plugin {
	mux := typemux.New(i)
	p := publishPlug{h: &mux}

	var publishMu sync.Mutex

	mux.RegisterAsync(p.Method(), &handler{
		info: i,

		publishMu: &publishMu,
		publish:   publish,
		authorLog: authorLog,

		boxer: boxer,
	})
	return p
}

func (p publishPlug) Name() string            { return "publish" }
func (p publishPlug) Method() muxrpc.Method   { return muxrpc.Method{"publish"} }
func (p publishPlug) Handler() muxrpc.Handler { return p.h }
