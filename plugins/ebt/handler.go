// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ebt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"

	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins/gossip"
	refs "go.mindeco.de/ssb-refs"
)

type idx2authorCacheMap map[refs.FeedRef]refs.FeedRef

type Replicate struct {
	info log.Logger

	self       refs.FeedRef
	receiveLog margaret.Log
	userFeeds  multilog.MultiLog

	// for metafeed lookups
	graph *graph.BadgerBuilder

	idx2authCacheMu sync.Mutex
	idx2authCache   idx2authorCacheMap

	livefeeds *gossip.FeedManager

	stateMatrix *statematrix.StateMatrix

	verify *message.VerificationRouter

	Sessions Sessions
}

func (h *Replicate) HandleLegacy(ctx context.Context, req *muxrpc.Request, src *muxrpc.ByteSource, snk *muxrpc.ByteSink) error {
	var args []struct {
		Version int
	}
	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		return err
	}

	if n := len(args); n != 1 {
		return fmt.Errorf("expected one argument but got %d", n)
	}
	arg := args[0]

	if arg.Version != 3 {
		return errors.New("go-ssb only supports ebt v3")
	}

	var format refs.RefAlgo = refs.RefAlgoFeedSSB1

	level.Debug(h.info).Log("event", "replicating", "version", arg.Version, "format", format)

	return h.Loop(ctx, snk, src, req.RemoteAddr(), format)
}

func (h *Replicate) HandleFormat(ctx context.Context, req *muxrpc.Request, src *muxrpc.ByteSource, snk *muxrpc.ByteSink) error {
	var args []struct {
		Version int
		Format  string
	}
	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		return err
	}

	if n := len(args); n != 1 {
		return fmt.Errorf("expected one argument but got %d", n)
	}
	arg := args[0]

	if arg.Version != 3 {
		return errors.New("go-ssb only supports ebt v3")
	}

	format := refs.RefAlgo(arg.Format)
	if err := ssb.IsValidFeedFormat(format); err != nil {
		return err
	}

	level.Debug(h.info).Log("event", "replicating", "version", arg.Version, "format", format)

	return h.Loop(ctx, snk, src, req.RemoteAddr(), format)
}

func (h *Replicate) Clock(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	peer, err := ssb.GetFeedRefFromAddr(req.RemoteAddr())
	if err != nil {
		return nil, err
	}

	var format refs.RefAlgo
	var args []struct {
		Format refs.RefAlgo
	}
	err = json.Unmarshal(req.RawArgs, &args)
	if err != nil || len(args) < 1 {
		format = refs.RefAlgoFeedSSB1
	} else {
		format = args[0].Format
	}

	nf, err := h.loadState(peer, format)
	if err != nil {
		return nil, err
	}
	return nf.Frontier, nil
}
