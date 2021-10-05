// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ebt

import (
	"fmt"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb/internal/statematrix"
	refs "go.mindeco.de/ssb-refs"
)

// newStateTrackingSink wraps a sink, for sending messages of a single author to a peer.
// it keeps track of each sent message using the passed stateMatrix and skips messages if they were already sent.
func newStateTrackingSink(stateMatrix *statematrix.StateMatrix, snk muxrpc.ByteSinker, peer, author refs.FeedRef, seq int64) muxrpc.ByteSinker {
	return &stateTrackingSink{
		ByteSinker: snk,

		stateMatrix: stateMatrix,

		peer: peer,

		author:   author,
		sequence: seq,
	}
}

type stateTrackingSink struct {
	// the sinker that sends to the network
	muxrpc.ByteSinker

	// peer with which peer we are communicating
	peer refs.FeedRef

	// which feed author we are tracking
	author refs.FeedRef

	// the current sequence
	sequence int64

	stateMatrix *statematrix.StateMatrix
}

// Write  updates the state matrix after sending to the network successfully
// it also checks if the peer still wants a message before sending it.
// This is a workaround to actually tracking and canceling live subscriptions for feeds.
func (sink *stateTrackingSink) Write(p []byte) (int, error) {
	wants, err := sink.stateMatrix.WantsFeedWithSeq(sink.peer, sink.author, sink.sequence)
	if err != nil {
		return -1, err
	}

	if !wants { // does not want? treat the write as a noop
		return 0, nil
	}

	// actually send the message
	n, err := sink.ByteSinker.Write(p)
	if err != nil {
		return -1, err
	}

	// increase our sequence of them
	sink.sequence++

	// update our perscived state
	err = sink.stateMatrix.UpdateSequences(sink.peer, []statematrix.FeedWithLength{
		{FeedRef: sink.author, Sequence: sink.sequence},
	})
	if err != nil {
		return -1, fmt.Errorf("stateTrackingSink: failed to update the matrix (%w)", err)
	}

	return n, nil
}

// these just call throuugh to the wrapped sink
