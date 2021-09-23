// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ebt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"go.cryptoscope.co/muxrpc/v2"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/message"
	gabbygrove "go.mindeco.de/ssb-gabbygrove"
	refs "go.mindeco.de/ssb-refs"
)

// Loop executes the ebt logic loop, reading from the peer and sending state and messages as requests
func (h *Replicate) Loop(ctx context.Context, tx *muxrpc.ByteSink, rx *muxrpc.ByteSource, remoteAddr net.Addr, format refs.RefAlgo) error {
	session := h.Sessions.Started(remoteAddr)

	peer, err := ssb.GetFeedRefFromAddr(remoteAddr)
	if err != nil {
		return err
	}

	peerLogger := log.With(h.info, "r", peer.ShortSigil(), "format", format)

	defer func() {
		h.Sessions.Ended(remoteAddr)

		level.Debug(peerLogger).Log("event", "loop exited")
		err := h.stateMatrix.SaveAndClose(peer)
		if err != nil {
			level.Warn(peerLogger).Log("event", "failed to save state matrix for peer", "err", err)
		}
	}()

	if err := h.sendState(tx, peer, format); err != nil {
		return err
	}

	// the buffer we will re-use to store incoming messages and notes
	buf := new(bytes.Buffer)

	// read/write loop for messages
	for rx.Next(ctx) {

		// always reset (we might 'continue' this loop)
		buf.Reset()

		// read the muxrpc frame into the buffer
		err := rx.Reader(func(r io.Reader) error {
			_, err := buf.ReadFrom(r)
			return err
		})
		if err != nil {
			return err
		}

		body := buf.Bytes()

		var frontierUpdate ssb.NetworkFrontier
		frontierUpdate.Format = format

		err = json.Unmarshal(body, &frontierUpdate)

		// assume it's a message if it fails to decode into a frontier
		if err != nil {

			// redundant pass of finding out the author
			// would be rad to get this from the pretty-printed version
			// and just pass that to verify (but also less generic)
			var author refs.FeedRef
			switch format {
			case refs.RefAlgoFeedSSB1:
				var msgWithAuthor struct {
					Author refs.FeedRef
				}

				err := json.Unmarshal(body, &msgWithAuthor)
				if err != nil {
					err = fmt.Errorf("unable to establish author for format %s: %w", format, err)
					level.Error(h.info).Log("error", err)
					continue
				}
				author = msgWithAuthor.Author

			case refs.RefAlgoFeedBendyButt:
				var msg metafeed.Message
				err = msg.UnmarshalBencode(body)
				if err != nil {
					err = fmt.Errorf("unable to establish author for format %s: %w", format, err)
					level.Error(h.info).Log("error", err)
					continue
				}

				author = msg.Author()

			case refs.RefAlgoFeedGabby:
				var msg gabbygrove.Transfer
				err = msg.UnmarshalCBOR(body)
				if err != nil {
					err = fmt.Errorf("unable to establish author for format %s: %w", format, err)
					level.Error(h.info).Log("error", err)
					continue
				}

				author = msg.Author()

			default:
				return fmt.Errorf("unhandled format: %s", format)
			}

			vsnk, err := h.verify.GetSink(author, true)
			if err != nil {
				level.Error(h.info).Log("error", err)
				continue
			}

			err = vsnk.Verify(body)
			if err != nil {
				// TODO: mark feed as bad
				level.Error(h.info).Log("error", err)
			}

			continue
		}

		// update our network perception
		wants, err := h.stateMatrix.Update(peer, &frontierUpdate)
		if err != nil {
			return err
		}

		filtered := filterRelevantNotes(wants, format, session.Unubscribe)

		// TODO: partition wants across the open connections
		// one peer might be closer to a feed
		// for this we also need timing and other heuristics

		// ad-hoc send where we have newer messages
		for feed, their := range filtered {
			arg := message.CreateHistArgs{
				ID:  feed,
				Seq: int64(their.Seq + 1),
			}
			arg.Limit = -1
			arg.Live = true

			// TODO: it might not scale to do this with contexts (each one has a goroutine)
			// in that case we need to rework the internal/luigiutils MultiSink so that we can unsubscribe on it directly
			ctx, cancel := context.WithCancel(ctx)

			// wrap the network output into a sink that tracks each write on the state matrix
			wrapped := newStateTrackingSink(h.stateMatrix, tx, peer, feed, their.Seq)

			err = h.livefeeds.CreateStreamHistory(ctx, wrapped, arg)
			if err != nil {
				cancel()
				return err
			}
			session.Subscribed(feed, cancel)
		}
	}

	return rx.Err()
}

func (h *Replicate) loadState(remote refs.FeedRef) (*ssb.NetworkFrontier, error) {
	currState, err := h.stateMatrix.Changed(h.self, remote)
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
	currState, err := h.loadState(remote)
	if err != nil {
		return err
	}

	currState.Format = format

	tx.SetEncoding(muxrpc.TypeJSON)
	// w := io.MultiWriter(tx, os.Stderr)
	w := tx
	err = json.NewEncoder(w).Encode(currState)
	if err != nil {
		return fmt.Errorf("failed to send currState: %d: %w", len(currState.Frontier), err)
	}

	return nil
}

func newStateTrackingSink(stateMatrix *statematrix.StateMatrix, snk muxrpc.ByteSinker, peer, author refs.FeedRef, seq int64) muxrpc.ByteSinker {
	return &stateTrackingSink{
		stateMatrix: stateMatrix,

		network: snk,
		peer:    peer,

		author:   author,
		sequence: seq,
	}
}

type stateTrackingSink struct {
	// peer with which peer we are communicating
	peer refs.FeedRef

	// the sinker that sends to the network
	network muxrpc.ByteSinker

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
	n, err := sink.network.Write(p)
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

func (st stateTrackingSink) Close() error {
	return st.network.Close()
}

func (st stateTrackingSink) CloseWithError(err error) error {
	return st.network.CloseWithError(err)
}

func (st stateTrackingSink) SetEncoding(re muxrpc.RequestEncoding) {
	st.network.SetEncoding(re)
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
