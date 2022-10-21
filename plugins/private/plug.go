// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// Package private suuplies an about to be deprecated way of accessing private messages.
// This supplies the private.read source from npm:ssb-private.
// In the  future relevant queries will have a private:true parameter to unbox readable messages automatically.
package private

import (
	"github.com/ssbc/margaret"
	"github.com/ssbc/go-muxrpc/v2"
	"github.com/ssbc/go-muxrpc/v2/typemux"
	"github.com/ssbc/go-ssb"
	"github.com/ssbc/go-ssb/private"
	"go.mindeco.de/logging"
	refs "github.com/ssbc/go-ssb-refs"
)

type privatePlug struct {
	h muxrpc.Handler
}

func NewPlug(i logging.Interface, author refs.FeedRef, mngr *private.Manager, publish ssb.Publisher, readIdx margaret.Log) ssb.Plugin {
	handler := handler{
		author:  author,
		mngr:    mngr,
		publish: publish,
		read:    readIdx,
		info:    i,
	}

	tm := typemux.New(i)

	tm.RegisterAsync(append(methodName, "publish"), typemux.AsyncFunc(handler.handlePublish))
	tm.RegisterSource(append(methodName, "read"), typemux.SourceFunc(handler.handleRead))

	return &privatePlug{h: &tm}
}

var methodName = muxrpc.Method{"private"}

func (p privatePlug) Name() string {
	return "private"
}

func (p privatePlug) Method() muxrpc.Method {
	return methodName
}

func (p privatePlug) Handler() muxrpc.Handler {
	return p.h
}
