package ebt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/numberedfeeds"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins/gossip"
	refs "go.mindeco.de/ssb-refs"
)

type MUXRPCHandler struct {
	info   logging.Interface
	isServ bool

	id        *refs.FeedRef
	rootLog   margaret.Log
	userFeeds multilog.MultiLog

	livefeeds *gossip.FeedManager

	wantList ssb.ReplicationLister

	feedNumbers *numberedfeeds.Index
	stateMatrix *statematrix.StateMatrix

	currentMessages map[string]refs.Message

	verify *message.VerifySink
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

	// TODO: check protocol version option
	h.info.Log("debug", "called", "args", string(req.RawArgs))

	remote, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		return
	}

	snk, err := req.GetResponseSink()
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		return
	}

	src, err := req.GetResponseSource()
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		return
	}

	h.Loop(ctx, snk, src, remote)
	h.info.Log("debug", "loop exited", "r", remote.ShortRef())
}

func (h MUXRPCHandler) sendState(ctx context.Context, tx *muxrpc.ByteSink, remote *refs.FeedRef) error {
	currState, err := h.stateMatrix.Changed(h.id, remote)
	if err != nil {
		return fmt.Errorf("failed to get changed frontier: %w", err)
	}

	if len(currState) == 0 { // no state yet
		lister := h.wantList.ReplicationList()
		feeds, err := lister.List()
		if err != nil {
			return fmt.Errorf("failed to get userlist: %w", err)
		}

		// TODO: see if they changed and if they want them
		for i, feed := range feeds {

			// filter the ones that didnt change

			seq, err := h.currentSequence(feed)
			if err != nil {
				return fmt.Errorf("failed to get sequence for entry %d: %w", i, err)
			}
			currState[feed] = seq
		}

		currState[h.id], err = h.currentSequence(h.id)
		if err != nil {
			return fmt.Errorf("failed to get our sequence: %w", err)
		}
	}

	fmt.Println("our state")
	fmt.Println(currState.String())

	err = json.NewEncoder(tx).Encode(currState)
	if err != nil {
		return fmt.Errorf("failed to send currState: %d: %w", len(currState), err)
	}

	return nil
}

func (h *MUXRPCHandler) Loop(ctx context.Context, tx *muxrpc.ByteSink, rx *muxrpc.ByteSource, remote *refs.FeedRef) {
	if err := h.sendState(ctx, tx, remote); err != nil {
		h.check(err)
		return
	}

	for rx.Next(ctx) { // read/write loop for messages

		jsonBody, err := rx.Bytes()
		if err != nil {
			h.check(err)
			return
		}

		var nf ssb.NetworkFrontier
		err = json.Unmarshal(jsonBody, &nf)
		if err != nil { // assume it's a message

			var msgWithAuthor struct {
				Author *refs.FeedRef
			}

			err := json.Unmarshal(jsonBody, &msgWithAuthor)
			if err != nil {
				h.check(err)
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

		fmt.Println("their state:", remote.ShortRef())
		fmt.Println(nf.String())

		// update our network perception
		var observed []statematrix.ObservedFeed

		// ad-hoc send where we have newer messages
		for feed, their := range nf {
			if !their.Replicate {
				observed = append(observed, statematrix.ObservedFeed{
					Feed:      feed,
					Replicate: false,
				})
				continue
			}

			ourState, err := h.currentSequence(feed)
			if err != nil {
				continue
			}

			if their.Receive && ourState.Seq > their.Seq { // we have more for them

				arg := &message.CreateHistArgs{
					ID:  feed.Copy(),
					Seq: their.Seq,
				}
				arg.Limit = -1
				arg.Live = true

				// TODO: need to change the fetch code
				err = h.livefeeds.CreateStreamHistory(ctx, tx, arg)
				if err != nil {
					h.check(err)
					return
				}
			}
		}

		h.stateMatrix.Fill(remote, observed)
		if err != nil {
			h.check(err)
			return
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
