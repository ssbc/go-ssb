// SPDX-License-Identifier: MIT

// Package publish is just a muxrpc wrapper around sbot.PublishLog.Publish.
package publish

import (
	"sync"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/muxmux"
	"go.cryptoscope.co/ssb/private"
)

type publishPlug struct{ h muxrpc.Handler }

func NewPlug(
	i logging.Interface,
	publish ssb.Publisher,
	boxer *private.Manager,
	authorLog margaret.Log,
) ssb.Plugin {
	mux := muxmux.New(i)
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
