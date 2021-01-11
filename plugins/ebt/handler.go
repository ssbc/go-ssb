// SPDX-License-Identifier: MIT

package ebt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins/gossip"
	refs "go.mindeco.de/ssb-refs"
)

type MUXRPCHandler struct {
	info logging.Interface

	self      *refs.FeedRef
	rootLog   margaret.Log
	userFeeds multilog.MultiLog

	livefeeds *gossip.FeedManager

	wantList ssb.ReplicationLister

	stateMatrix *statematrix.StateMatrix

	verify *message.VerifySink

	Sessions Sessions
}

func (h *MUXRPCHandler) check(err error) {
	if err != nil {
		level.Error(h.info).Log("error", err)
	}
}

// HandleConnect does nothing. Feature negotiation is done by sbot
func (h *MUXRPCHandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

// HandleCall handles the server side (getting called by client)
func (h *MUXRPCHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	var closed bool
	checkAndClose := func(err error) {
		h.check(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			h.check(fmt.Errorf("error closeing request %q: %w", req.Method, closeErr))
		}
	}

	defer func() {
		if !closed {
			cerr := req.Stream.Close()
			if cerr != nil {
				h.check(fmt.Errorf("gossip: error closing call %q: %w", req.Method, cerr))
			}
		}
	}()

	if req.Method.String() != "ebt.replicate" {
		checkAndClose(fmt.Errorf("unknown command: %s", req.Method))
		return
	}

	if req.Type != "duplex" {
		checkAndClose(fmt.Errorf("invalid type: %s", req.Type))
		return
	}

	var args []struct{ Version int }
	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		checkAndClose(err)
		return
	}

	if n := len(args); n != 1 {
		checkAndClose(fmt.Errorf("expected one argument but got %d", n))
		return
	}

	h.info.Log("debug", "called", "args", args[0])
	if args[0].Version != 3 {
		checkAndClose(errors.New("go-ssb only support ebt v3"))
		return
	}

	// get writer and reader from duplex call
	snk, err := req.GetResponseSink()
	if err != nil {
		checkAndClose(err)
		return
	}

	src, err := req.GetResponseSource()
	if err != nil {
		checkAndClose(err)
		return
	}

	remoteAddr := edp.Remote()
	h.Loop(ctx, snk, src, remoteAddr)
}

func (h MUXRPCHandler) sendState(ctx context.Context, tx *muxrpc.ByteSink, remote *refs.FeedRef) error {
	currState, err := h.stateMatrix.Changed(h.self, remote)
	if err != nil {
		return fmt.Errorf("failed to get changed frontier: %w", err)
	}

	selfRef := h.self.Ref()
	if len(currState) == 0 { // no state yet
		lister := h.wantList.ReplicationList()
		feeds, err := lister.List()
		if err != nil {
			return fmt.Errorf("failed to get userlist: %w", err)
		}

		for i, feed := range feeds {
			// filter the ones that didnt change
			seq, err := h.currentSequence(feed)
			if err != nil {
				return fmt.Errorf("failed to get sequence for entry %d: %w", i, err)
			}
			currState[feed.Ref()] = seq
		}

		currState[selfRef], err = h.currentSequence(h.self)
		if err != nil {
			return fmt.Errorf("failed to get our sequence: %w", err)
		}
	}

	// don't receive your own feed
	if myNote, has := currState[selfRef]; has {
		myNote.Receive = false
		currState[selfRef] = myNote
	}

	fmt.Printf("[%s] my state\n%s\n", h.self.ShortRef(), currState)
	tx.SetEncoding(muxrpc.TypeJSON)
	err = json.NewEncoder(tx).Encode(currState)
	if err != nil {
		return fmt.Errorf("failed to send currState: %d: %w", len(currState), err)
	}

	return nil
}

func (h *MUXRPCHandler) Loop(ctx context.Context, tx *muxrpc.ByteSink, rx *muxrpc.ByteSource, remoteAddr net.Addr) {
	h.Sessions.Started(remoteAddr)

	peer, err := ssb.GetFeedRefFromAddr(remoteAddr)
	if err != nil {
		h.check(err)
		return
	}

	defer func() {
		h.Sessions.Ended(remoteAddr)
		h.info.Log("debug", "loop exited", "r", peer.ShortRef())
	}()

	if err := h.sendState(ctx, tx, peer); err != nil {
		h.check(err)
		return
	}

	var buf = &bytes.Buffer{}
	for rx.Next(ctx) { // read/write loop for messages

		buf.Reset()
		err := rx.Reader(func(r io.Reader) error {
			_, err := buf.ReadFrom(r)
			return err
		})
		if err != nil {
			h.check(err)
			return
		}

		jsonBody := buf.Bytes()

		var frontierUpdate ssb.NetworkFrontier
		err = json.Unmarshal(jsonBody, &frontierUpdate)
		if err != nil { // assume it's a message

			// redundant pass of finding out the author
			// would be rad to get this from the pretty-printed version
			// and just pass that to verify
			var msgWithAuthor struct {
				Author *refs.FeedRef
			}

			err := json.Unmarshal(jsonBody, &msgWithAuthor)
			if err != nil {
				h.check(err)
				continue
			}
			fmt.Printf("[%s] new message from\n", msgWithAuthor.Author.Ref())

			if msgWithAuthor.Author == nil {
				fmt.Println("debug body:", string(jsonBody))
				h.check(fmt.Errorf("message without author?"))
				continue
			}

			vsnk, err := h.verify.GetSink(msgWithAuthor.Author)
			if err != nil {
				h.check(err)
				continue
			}

			err = vsnk.Verify(jsonBody)
			if err != nil {
				// TODO: mark feed as bad
				h.check(err)
			}

			continue
		}

		fmt.Printf("[%s] their state (from: %s)\n%s\n", h.self.ShortRef(), peer.Ref(), frontierUpdate)

		// update our network perception
		wants, err := h.stateMatrix.Update(peer, frontierUpdate)
		if err != nil {
			h.check(err)
			return
		}

		// ad-hoc send where we have newer messages
		for feedStr, their := range wants {
			// these were already validated by the .UnmarshalJSON() method
			// but we need the refs.Feed for the createHistArgs
			feed, err := refs.ParseFeedRef(feedStr)
			if err != nil {
				h.check(err)
				return
			}

			if !their.Replicate {
				continue
			}

			// TODO: don't double subscribe
			if their.Receive {
				arg := &message.CreateHistArgs{
					ID:  feed,
					Seq: their.Seq + 1,
				}
				arg.Limit = -1
				arg.Live = true

				err = h.livefeeds.CreateStreamHistory(ctx, tx, arg)
				if err != nil {
					h.check(err)
					return
				}
			}
		}
	}

	h.check(rx.Err())
}

// utils

func (h MUXRPCHandler) currentSequence(feed *refs.FeedRef) (ssb.Note, error) {
	l, err := h.userFeeds.Get(storedrefs.Feed(feed))
	if err != nil {
		return ssb.Note{}, fmt.Errorf("failed to get user log for %s: %w", feed.ShortRef(), err)
	}
	sv, err := l.Seq().Value()
	if err != nil {
		return ssb.Note{}, fmt.Errorf("failed to get sequence for user log %s: %w", feed.ShortRef(), err)
	}

	return ssb.Note{
		Seq:       sv.(margaret.BaseSeq).Seq() + 1,
		Replicate: true,
		Receive:   true, // TODO: not exactly... we might be getting this feed from somewhre else
	}, nil
}
