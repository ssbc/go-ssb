package ebt

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"go.cryptoscope.co/ssb/message/legacy"

	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
)

type handler struct {
	info   logging.Interface
	isServ bool

	id        *refs.FeedRef
	rootLog   margaret.Log
	userFeeds multilog.MultiLog

	wantList ssb.ReplicationLister
}

func New(i logging.Interface, id *refs.FeedRef, rootLog margaret.Log, userFeeds multilog.MultiLog, wantList ssb.ReplicationLister) muxrpc.Handler {
	return &handler{
		info:      i,
		id:        id,
		rootLog:   rootLog,
		userFeeds: userFeeds,
		wantList:  wantList,
	}
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

	ref, err := ssb.GetFeedRefFromAddr(e.Remote())
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		// checkAndClose(err)
		return
	}
	h.info.Log("event", "triggering ebt.replicate", "r", ref.ShortRef())

	var opt = map[string]interface{}{
		"version": 3,
	}

	src, snk, err := e.Duplex(ctx, json.RawMessage{}, muxrpc.Method{"ebt", "replicate"}, opt)
	h.info.Log("event", "call replicate", "err", err)
	if err != nil {
		return
	}
	go h.sendTo(ctx, snk, ref)
	h.readFrom(ctx, src)

}

func (h *handler) sendTo(ctx context.Context, tx luigi.Sink, remote *refs.FeedRef) {
	// defer tx.Close()
	err := h.sendState(ctx, tx)
	if err != nil {
		h.check(err)
		return
	}
}

type networkFrontier map[*refs.FeedRef]uint64

func (nf *networkFrontier) UnmarshalJSON(b []byte) error {
	var dummy map[string]uint64

	if err := json.Unmarshal(b, &dummy); err != nil {
		return err
	}

	var newMap = make(networkFrontier, len(dummy))
	for fstr, seq := range dummy {
		ref, err := refs.ParseFeedRef(fstr)
		if err != nil {
			return err
		}

		newMap[ref] = seq
	}

	*nf = newMap
	return nil
}

func (nf networkFrontier) String() string {
	var sb strings.Builder
	sb.WriteString("## Network Frontier:\n")
	for feed, seq := range nf {
		fmt.Fprintf(&sb, "\t%s:%d\n", feed.ShortRef(), seq)
	}
	return sb.String()
}

func (h *handler) readFrom(ctx context.Context, rx luigi.Source) {
	for {
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

		var nf networkFrontier
		err = json.Unmarshal(jsonBody, &nf)
		if err != nil {
			// check as message
			ref, desr, err2 := legacy.Verify(jsonBody, nil)
			if err2 != nil {
				fmt.Println(err.Error())
				h.check(err2)
				return
			}
			fmt.Println(desr.Author.ShortRef(), desr.Sequence, ref.ShortRef())
			continue
		}

		fmt.Println("their state:")
		fmt.Println(nf.String())

	}
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

	err := h.sendState(ctx, req.Stream)
	if err != nil {
		h.info.Log("event", "sendState failed", "err", err)
		checkAndClose(err)
		return
	}

	ref, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		h.info.Log("event", "call replicate", "err", err)
		checkAndClose(err)
		return
	}

	v, err := req.Stream.Next(ctx)
	if err != nil {
		h.info.Log("err", err, "msg", "did not get ebt state")
		if luigi.IsEOS(err) {
			return
		}
		return
	}
	// h.info.Log("debug", "got ebt state", "type", fmt.Sprintf("%T", v))
	// goon.Dump(v)

	b, err := json.Marshal(v)
	if err != nil {
		h.info.Log("err", err, "msg", "json dump failed state")
		return
	}
	ioutil.WriteFile(fmt.Sprintf("ebt-%x.json", ref.ID), b, 0700)
}

func (h handler) sendState(ctx context.Context, snk luigi.Sink) error {
	currState := make(networkFrontier)

	lister := h.wantList.ReplicationList()
	feeds, err := lister.List()
	if err != nil {
		return errors.Wrap(err, "failed to get userlist")
	}

	// TODO: see if they changed and if they want them
	for i, feed := range feeds {
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

	fmt.Println("our state")
	fmt.Println(currState.String())

	// b, err := json.Marshal(currState)
	// if err != nil {
	// 	h.info.Log("err", err, "msg", "json dump failed state")
	// } else {
	// 	ioutil.WriteFile(fmt.Sprintf("ebt-%x.json", h.id), b, 0700)
	// }

	err = snk.Pour(ctx, currState)
	if err != nil {
		return errors.Wrapf(err, "failed to send currState: %d", len(currState))
	}

	// mylog, err := h.userFeeds.Get(librarian.Addr(h.id.ID))
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to open mylog")
	// }

	// src, err := mylog.Query()
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to open mylog")
	// }

	// err = luigi.Pump(ctx, snk, src)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to copy mylog to remote")
	// }
	// time.Sleep(2 * time.Second)

	return nil
}

func (h handler) currentSequence(feed *refs.FeedRef) (uint64, error) {
	l, err := h.userFeeds.Get(feed.StoredAddr())
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get user log %s", feed.ShortRef())
	}
	sv, err := l.Seq().Value()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get sequence for user log:%s", feed.ShortRef())
	}

	return uint64(sv.(margaret.BaseSeq) + 1), nil
}
