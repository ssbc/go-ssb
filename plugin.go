// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"net"
	"sync"

	"go.cryptoscope.co/muxrpc/v2"
)

// Plugin represents a muxrpc handler plugin in go-ssb
type Plugin interface {
	// Name returns the name and version of the plugin.
	// format: name-1.0.2
	Name() string

	// Method returns the preferred method of the call
	Method() muxrpc.Method

	// Handler returns the muxrpc handler for the plugin
	Handler() muxrpc.Handler
}

// PluginManager allows for handler registration and creating handlers from network connections
type PluginManager interface {
	Register(Plugin)
	MakeHandler(conn net.Conn) (muxrpc.Handler, error)
}

type pluginManager struct {
	regLock sync.Mutex // protects the map
	plugins map[string]Plugin
}

// NewPluginManager returns a new PluginManager
func NewPluginManager() PluginManager {
	return &pluginManager{
		plugins: make(map[string]Plugin),
	}
}

func (pmgr *pluginManager) Register(p Plugin) {
	//  access race
	pmgr.regLock.Lock()
	defer pmgr.regLock.Unlock()
	pmgr.plugins[p.Method().String()] = p
}

func (pmgr *pluginManager) MakeHandler(conn net.Conn) (muxrpc.Handler, error) {
	// TODO: add authorization requirements check to plugin so we can call it here
	// e.g. only allow some peers to make certain requests

	pmgr.regLock.Lock()
	defer pmgr.regLock.Unlock()

	h := muxrpc.HandlerMux{}

	// var hs []muxrpc.NamedHandler
	for _, p := range pmgr.plugins {
		h.Register(p.Method(), p.Handler())
		// hs = append(hs, muxrpc.NamedHandler{p.Method(), p.Handler()})
	}
	// h.RegisterAll(hs...)

	return &h, nil
}
