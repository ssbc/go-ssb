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
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/numberedfeeds"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/message/legacy"
	refs "go.mindeco.de/ssb-refs"
)

type handler struct {
	info   logging.Interface
	isServ bool

	id        *refs.FeedRef
	rootLog   margaret.Log
	userFeeds multilog.MultiLog

	wantList ssb.ReplicationLister

	feedNumbers *numberedfeeds.Index
	stateMatrix *statematrix.StateMatrix

	currentMessages map[string]refs.Message
}

func (h *handler) check(err error) {
	if err != nil {
		level.Error(h.info).Log("error", err)
	}
}

// client side, asking remote for support
// TODO: we need to coordinate this with legacy gossip
func (h *handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	// TODO: find a way to signal if we are client or server

	remote, err := ssb.GetFeedRefFromAddr(e.Remote())
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		return
	}

	h.info.Log("event", "triggering ebt.replicate", "r", remote.ShortRef())

	var opt = map[string]interface{}{"version": 3}

	// initiate ebt channel
	rx, tx, err := e.Duplex(ctx, json.RawMessage{}, muxrpc.Method{"ebt", "replicate"}, opt)
	h.info.Log("event", "call replicate", "err", err)
	if err != nil {
		return
	}

	h.loop(ctx, tx, rx, remote)
}

// server side (getting called by client asking for support)
func (h *handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
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

	h.loop(ctx, req.Stream, req.Stream, remote)
	h.info.Log("debug", "loop exited", "r", remote.ShortRef())

}

func (h handler) sendState(ctx context.Context, snk luigi.Sink, remote *refs.FeedRef) error {

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

func (h *handler) loop(ctx context.Context, tx luigi.Sink, rx luigi.Source, remote *refs.FeedRef) {
	sentAsJSON := transform.NewKeyValueWrapper(tx, false)

	/*
		sv, err := h.rootLog.Seq().Value()
		if err != nil {
			panic(errors.Wrap(err, "failed to get current rx log sequence"))
		}
		newMsgs, err := h.rootLog.Query(margaret.Live(true), margaret.Gt(sv.(margaret.Seq)))
		if err != nil {
			panic(errors.Wrap(err, "failed to get current rx log sequence"))
		}

		updateSink := luigi.FuncSink(func(notifyCtx context.Context, v interface{}, closed error) error {
			if closed != nil {
				if luigi.IsEOS(closed) {
					return nil
				}
				level.Error(h.info).Log("newMsg-closed", closed)
				return closed
			}

			msg := v.(refs.Message)

			yes, err := h.stateMatrix.WantsFeed(remote, msg.Author(), uint64(msg.Seq()))
			if err != nil {
				level.Error(h.info).Log("newMsg-queryerr", err, "seq", msg.Seq())
				return err
			}
			if yes {
				level.Info(h.info).Log("newMsg", msg.Key().ShortRef(), "seq", msg.Seq())
				err = sentAsJSON.Pour(ctx, msg)
				if err != nil {
					level.Error(h.info).Log("sent-new-err", err, "remote", remote.ShortRef())
					return nil
				}
			}

			return nil
		})
		go luigi.Pump(ctx, updateSink, newMsgs)
	*/

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

		jsonBody, ok := v.(json.RawMessage)
		if !ok {
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
				log, err := h.userFeeds.Get(feed.StoredAddr())
				if err != nil {
					h.check(err)
					return
				}

				src, err := mutil.Indirect(h.rootLog, log).Query(margaret.Gte(margaret.BaseSeq(their.Seq)))
				if err != nil {
					h.check(err)
					return
				}

				err = luigi.Pump(ctx, sentAsJSON, src)
				if err != nil {
					h.check(err)
					return
				}
				their.Seq = ourState.Seq // we sent all ours. assuming they received it
			}

			observed = append(observed, statematrix.ObservedFeed{
				Feed:      feed,
				Replicate: their.Replicate,
				Receive:   their.Receive,
				Len:       uint64(their.Seq),
			})
		}

		h.stateMatrix.Fill(remote, observed)
		if err != nil {
			h.check(err)
			return
		}
	}
}

// utils

func (h handler) currentSequence(feed *refs.FeedRef) (ssb.Note, error) {
	l, err := h.userFeeds.Get(feed.StoredAddr())
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
