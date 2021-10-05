// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ebt

import (
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

// wraps a sink and changes the write call such that it
func (r *Replicate) indexedWrapper(snk muxrpc.ByteSinker, author refs.FeedRef) (muxrpc.ByteSinker, error) {
	authorIndex, err := r.userFeeds.Get(storedrefs.Feed(author))
	if err != nil {
		return nil, err
	}

	return &indexedSender{
		ByteSinker: snk,
		actualLog:  mutil.Indirect(r.receiveLog, authorIndex),
	}, nil
}

type indexedSender struct {
	// the sinker that sends to the network
	muxrpc.ByteSinker

	// the full messages for this author
	actualLog margaret.Log
}

func (sink *indexedSender) Write(p []byte) (int, error) {
	var indexedMsg ssb.IndexedMessage
	err := json.Unmarshal(p, &indexedMsg)
	if err != nil {
		return -1, err
	}

	actualMsgV, err := sink.actualLog.Get(indexedMsg.Indexed.Sequence)
	if err != nil {
		return -1, err
	}
	actualMsg := actualMsgV.(refs.Message)

	if !actualMsg.Key().Equal(indexedMsg.Indexed.Key) {
		return -1, fmt.Errorf("indexed message does not reference author message as expected")
	}

	// now stitch indexed and acutal message together
	// as [actual, indexed], ....
	stitchedMsg := []byte{'['}
	stitchedMsg = append(stitchedMsg, p...)
	stitchedMsg = append(stitchedMsg, ',')
	stitchedMsg = append(stitchedMsg, actualMsg.ValueContentJSON()...)
	stitchedMsg = append(stitchedMsg, ']')

	return sink.ByteSinker.Write(stitchedMsg)
}
