// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// Package replicate roughly translates to npm:ssb-replicate and only selects which feeds to block and fetch.
//
// TODO: move ctrl.replicate and ctrl.block here.
package replicate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ssbc/margaret/multilog"
	"github.com/ssbc/go-muxrpc/v2"
	"github.com/ssbc/go-muxrpc/v2/typemux"
	"go.mindeco.de/log"
	refs "github.com/ssbc/go-ssb-refs"

	"github.com/ssbc/go-ssb"
)

type replicatePlug struct {
	h muxrpc.Handler
}

// TODO: add request, block, changes
func NewPlug(users multilog.MultiLog, self refs.FeedRef, lister ssb.ReplicationLister) ssb.Plugin {
	plug := &replicatePlug{}

	tm := typemux.New(log.NewNopLogger())

	tm.RegisterSource(muxrpc.Method{"replicate", "upto"}, replicateHandler{
		users:  users,
		wanted: lister,
		self:   self,
	})

	plug.h = &tm
	return plug
}

func (lt replicatePlug) Name() string { return "replicate" }

func (replicatePlug) Method() muxrpc.Method {
	return muxrpc.Method{"replicate"}
}
func (lt replicatePlug) Handler() muxrpc.Handler {
	return lt.h
}

type replicateHandler struct {
	users  multilog.MultiLog
	self   refs.FeedRef
	wanted ssb.ReplicationLister
}

func (g replicateHandler) HandleSource(ctx context.Context, req *muxrpc.Request, sink *muxrpc.ByteSink) error {
	wantedSet := g.wanted.ReplicationList()
	wantedSet.AddRef(g.self)
	list, err := wantedSet.List()
	if err != nil {
		return err
	}

	set, err := ssb.WantedFeedsWithSeqs(g.users, list)
	if err != nil {
		return fmt.Errorf("replicate: did not get feed source: %w", err)
	}

	sink.SetEncoding(muxrpc.TypeJSON)
	enc := json.NewEncoder(sink)

	for _, resp := range set {
		err = enc.Encode(resp)
		if err != nil {
			return err
		}
	}

	return sink.Close()
}
