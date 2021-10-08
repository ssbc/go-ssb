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
	peerLogger := log.With(h.info, "peer", peer.ShortSigil(), "format", format)

	defer func() {
		h.Sessions.Ended(remoteAddr)

		level.Debug(peerLogger).Log("event", "loop exited")
		err := h.stateMatrix.SaveAndClose(peer, format)
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

		// update our perception of the network with the new frontier
		wants, err := h.stateMatrix.Update(peer, &frontierUpdate, format)
		if err != nil {
			return err
		}

		filterFormat := format
		if format == "indexed" {
			filterFormat = refs.RefAlgoFeedSSB1
		}
		filtered := filterRelevantNotes(wants, filterFormat, session.Unubscribe)

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

			var sender muxrpc.ByteSinker = tx

			// in indexed mode we need to tune the sender such that it merges the indexed with the referenced message
			if format == "indexed" {
				authorForIndex, err := h.findAuthorForIndexfeed(ctx, feed)
				if err != nil {
					level.Warn(peerLogger).Log("event", "no author for index feed", "err", err)
				} else {
					sender, err = h.indexedWrapper(sender, authorForIndex)
					if err != nil {
						return err
					}
				}
			}

			// wrap the network output into a sink that tracks each write on the state matrix
			trackingSink := newStateTrackingSink(h.stateMatrix, sender, peer, feed, their.Seq)

			// TODO: it might not scale to do this with contexts (each one has a goroutine)
			// in that case we need to rework the internal/luigiutils MultiSink so that we can unsubscribe on it directly
			ctx, cancel := context.WithCancel(ctx)

			err = h.livefeeds.CreateStreamHistory(ctx, trackingSink, arg)
			if err != nil {
				cancel()
				return err
			}
			session.Subscribed(feed, cancel)
		}
	}

	return rx.Err()
}
