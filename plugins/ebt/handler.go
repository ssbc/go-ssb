package ebt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/numberedfeeds"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/message/legacy"
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
}

func (h *MUXRPCHandler) check(err error) {
	if err != nil {
		level.Error(h.info).Log("error", err)
	}
}

// client side, asking remote for support
// TODO: we need to coordinate this with legacy gossip
func (h *MUXRPCHandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {

}

// server side (getting called by client asking for support)
func (h *MUXRPCHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	var closed bool
	checkAndClose := func(err error) {
		h.check(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			h.check(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	defer func() {
		if !closed {
			h.check(errors.Wrapf(req.Stream.Close(), "gossip: error closing call: %s", req.Method))
		}
	}()

	if req.Method.String() != "ebt.replicate" {
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
		return
	}

	if req.Type != "duplex" {
		checkAndClose(errors.Errorf("invalid type: %s", req.Type))
		return
	}

	// TODO: check protocol version option
	h.info.Log("debug", "called", "args", fmt.Sprintf("%+v", req.Args))

	remote, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		return
	}

	h.Loop(ctx, req.Stream, req.Stream, remote)
	h.info.Log("debug", "loop exited", "r", remote.ShortRef())
}

func (h MUXRPCHandler) sendState(ctx context.Context, snk luigi.Sink, remote *refs.FeedRef) error {

	currState, err := h.stateMatrix.Changed(h.id, remote)
	if err != nil {
		return errors.Wrap(err, "failed to get changed frontier")
	}

	if len(currState) == 0 { // no state yet
		lister := h.wantList.ReplicationList()
		feeds, err := lister.List()
		if err != nil {
			return errors.Wrap(err, "failed to get userlist")
		}

		// TODO: see if they changed and if they want them
		for i, feed := range feeds {

			// filter the ones that didnt change

			seq, err := h.currentSequence(feed)
			if err != nil {
				return errors.Wrapf(err, "failed to get sequence for entry %d", i)
			}
			currState[feed] = seq
		}

		currState[h.id], err = h.currentSequence(h.id)
		if err != nil {
			return errors.Wrap(err, "failed to get our sequence")
		}
	}

	fmt.Println("our state")
	fmt.Println(currState.String())

	err = snk.Pour(ctx, currState)
	if err != nil {
		return errors.Wrapf(err, "failed to send currState: %d", len(currState))
	}

	return nil
}

func (h *MUXRPCHandler) Loop(ctx context.Context, tx luigi.Sink, rx luigi.Source, remote *refs.FeedRef) {
	sentAsJSON := transform.NewKeyValueWrapper(tx, false)

	if err := h.sendState(ctx, tx, remote); err != nil {
		h.check(err)
		return
	}

	for { // read/write loop for messages

		v, err := rx.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			h.check(err)
			return
		}

		var jsonBody []byte
		switch tv := v.(type) {
		case json.RawMessage:
			jsonBody = tv
		case map[string]interface{}:

			jsonBody, err = json.Marshal(tv)
			if err != nil {
				h.check(err)
				return
			}
			fmt.Println("warning: remarshaling json - breaks signtures")
			bs := string(jsonBody)
			if len(bs) > 30 {
				bs = bs[:25] + "..."
			}
			fmt.Println(bs)
		default:
			panic(fmt.Sprintf("wrong type: %T", v))
		}

		var nf ssb.NetworkFrontier
		err = json.Unmarshal(jsonBody, &nf)
		if err != nil { // check as message

			// verify and validate next message and append it to the receive log

			// TODO: format support

			// TODO: hmac setup
			ref, desr, err2 := legacy.Verify(jsonBody, nil)
			if err2 != nil {
				// TODO: mark feed as bad
				h.check(err2)
				continue
			}

			nextMsg := &legacy.StoredMessage{
				Author_:    &desr.Author,
				Previous_:  desr.Previous,
				Key_:       ref,
				Sequence_:  desr.Sequence,
				Timestamp_: time.Now(),
				Raw_:       jsonBody,
			}

			current := h.currentMessages[desr.Author.Ref()]
			if err := message.ValidateNext(current, nextMsg); err != nil {
				level.Debug(h.info).Log("current", current.Seq(), "next", nextMsg.Seq(), "err", err)
				continue
			}
			h.currentMessages[desr.Author.Ref()] = nextMsg

			seq, err := h.rootLog.Append(nextMsg)
			if err != nil {
				h.check(err)
				return
			}
			level.Debug(h.info).Log("author", nextMsg.Author().ShortRef(), "got", nextMsg.Seq(), "rxlog", seq)

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

				err = h.livefeeds.CreateStreamHistory(ctx, sentAsJSON, arg)
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
}

// utils

func (h MUXRPCHandler) currentSequence(feed *refs.FeedRef) (ssb.Note, error) {
	l, err := h.userFeeds.Get(storedrefs.Feed(feed))
	if err != nil {
		return ssb.Note{}, errors.Wrapf(err, "failed to get user log %s", feed.ShortRef())
	}
	sv, err := l.Seq().Value()
	if err != nil {
		return ssb.Note{}, errors.Wrapf(err, "failed to get sequence for user log:%s", feed.ShortRef())
	}

	return ssb.Note{
		Seq:       sv.(margaret.BaseSeq).Seq() + 1,
		Replicate: true,
		Receive:   true, // TODO: not exactly... we might be getting this feed from somewhre else
	}, nil
}
