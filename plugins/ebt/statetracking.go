// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ebt

import (
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/statematrix"
	refs "go.mindeco.de/ssb-refs"
)

func (h *Replicate) loadState(remote refs.FeedRef, format refs.RefAlgo) (*ssb.NetworkFrontier, error) {
	currState, err := h.stateMatrix.Changed(h.self, remote, format)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed frontier: %w", err)
	}

	selfRef := h.self.String()

	// don't receive your own feed
	if myNote, has := currState.Frontier[selfRef]; has {
		myNote.Receive = false
		currState.Frontier[selfRef] = myNote
	}

	return currState, nil
}

func (h *Replicate) sendState(tx *muxrpc.ByteSink, remote refs.FeedRef, format refs.RefAlgo) error {
	currState, err := h.loadState(remote, format)
	if err != nil {
		return err
	}

	tx.SetEncoding(muxrpc.TypeJSON)
	err = json.NewEncoder(tx).Encode(currState)
	if err != nil {
		return fmt.Errorf("failed to send currState: %d: %w", len(currState.Frontier), err)
	}

	return nil
}

type filteredNotes map[refs.FeedRef]ssb.Note

// creates a new map with relevant feeds to remove the lock on the input quickly
func filterRelevantNotes(all *ssb.NetworkFrontier, format refs.RefAlgo, unsubscribe func(refs.FeedRef)) filteredNotes {
	filtered := make(filteredNotes)

	// take the map now for a swift copy of the map
	// then we don't have to hold it while doing the (slow) i/o
	all.Lock()
	defer all.Unlock()

	for feedStr, their := range all.Frontier {
		// these were already validated by the .UnmarshalJSON() method
		// but we need the refs.Feed for the createHistArgs
		feed, err := refs.ParseFeedRef(feedStr)
		if err != nil {
			panic(err)
		}

		// skip feeds not in the desired format for this sessions
		if feed.Algo() != format {
			continue
		}

		if !their.Replicate {
			unsubscribe(feed)
			continue
		}

		if !their.Receive {
			unsubscribe(feed)
			continue
		}

		filtered[feed] = their
	}

	return filtered
}

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
