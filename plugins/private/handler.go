// SPDX-License-Identifier: MIT

package private

import (
	"context"
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.mindeco.de/logging"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/private"
	refs "go.mindeco.de/ssb-refs"
)

type handler struct {
	info logging.Interface

	author  refs.FeedRef
	publish ssb.Publisher
	read    margaret.Log

	mngr *private.Manager
}

func (handler) Handled(m muxrpc.Method) bool {
	if len(m) != 2 {
		return false
	}

	if m[0] != "private" {
		return false
	}

	return m[1] == "publish" || m[1] == "read"
}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request) {
	var closed bool
	checkAndClose := func(err error) {
		if err != nil {
			h.info.Log("event", "closing", "method", req.Method, "err", err)
		}
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			err := fmt.Errorf("error closeing request: %w", closeErr)
			if err != nil {
				h.info.Log("event", "closing", "method", req.Method, "err", err)
			}
		}
	}

	defer func() {
		if !closed {
			err := fmt.Errorf("gossip: error closing call: %w", req.Stream.Close())
			if err != nil {
				h.info.Log("event", "closing", "method", req.Method, "err", err)
			}
		}
	}()

	switch req.Method.String() {

	case "private.publish":
		if req.Type == "" {
			req.Type = "async"
		}
		if n := len(req.Args()); n != 2 {
			req.CloseWithError(fmt.Errorf("private/publish: bad request. expected 2 argument got %d", n))
			return
		}

		msg, err := json.Marshal(req.Args()[0])
		if err != nil {
			req.CloseWithError(fmt.Errorf("failed to encode message: %w", err))
			return
		}

		rcps, ok := req.Args()[1].([]interface{})
		if !ok {
			req.CloseWithError(fmt.Errorf("private/publish: wrong argument type. expected []strings but got %T", req.Args()[1]))
			return
		}

		rcpsRefs := make([]refs.FeedRef, len(rcps))
		for i, rv := range rcps {
			rstr, ok := rv.(string)
			if !ok {
				req.CloseWithError(fmt.Errorf("private/publish: wrong argument type. expected strings but got %T", rv))
				return
			}
			// TODO: box2 message ref
			rcpsRefs[i], err = refs.ParseFeedRef(rstr)
			if err != nil {
				req.CloseWithError(fmt.Errorf("private/publish: failed to parse recp %d: %w", i, err))
				return
			}
		}

		ref, err := h.privatePublish(msg, rcpsRefs)
		if err != nil {
			req.CloseWithError(err)
			return
		}

		err = req.Return(ctx, ref)
		if err != nil {
			h.info.Log("event", "error", "msg", "cound't return new msg ref")
			return
		}
		h.info.Log("published", ref.Ref())

		return

	case "private.read":
		if req.Type != "source" {
			checkAndClose(fmt.Errorf("private.read: wrong request type. %s", req.Type))
			return
		}
		h.privateRead(ctx, req)

	default:
		checkAndClose(fmt.Errorf("private: unknown command: %s", req.Method))
	}
}

func (h handler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

func (h handler) privateRead(ctx context.Context, req *muxrpc.Request) {
	var qry message.CreateHistArgs

	args := req.Args()
	if len(args) > 0 {

		switch v := args[0].(type) {
		case map[string]interface{}:
			q, err := message.NewCreateHistArgsFromMap(v)
			if err != nil {
				req.CloseWithError(fmt.Errorf("privateRead: bad request: %w", err))
				return
			}
			qry = *q
		default:
			req.CloseWithError(fmt.Errorf("privateRead: invalid argument type %T", args[0]))
			return
		}

		if qry.Live {
			qry.Limit = -1
		}
	} else {
		qry.Limit = -1
	}

	// well, sorry - the client lib needs better handling of receiving types
	qry.Keys = true

	src, err := h.read.Query(
		margaret.Gte(margaret.BaseSeq(qry.Seq)),
		margaret.Limit(int(qry.Limit)),
		margaret.Live(qry.Live))
	if err != nil {
		req.CloseWithError(fmt.Errorf("private/read: failed to create query: %w", err))
		return
	}

	snk, err := req.ResponseSink()
	if err != nil {
		req.CloseWithError(err)
		return
	}

	err = luigi.Pump(ctx, transform.NewKeyValueWrapper(snk, qry.Keys), src)
	if err != nil {
		req.CloseWithError(fmt.Errorf("private/read: message pump failed: %w", err))
		return
	}
	req.Close()
}

func (h handler) privatePublish(msg []byte, recps []refs.FeedRef) (refs.MessageRef, error) {

	boxedMsg, err := h.mngr.EncryptBox1(msg, recps...)
	if err != nil {
		return refs.MessageRef{}, fmt.Errorf("private/publish: failed to box message: %w", err)
	}

	if h.author.Algo() == refs.RefAlgoFeedGabby {
		boxedMsg = append([]byte("box1:"), boxedMsg...)
	}

	ref, err := h.publish.Publish(boxedMsg)
	if err != nil {
		return refs.MessageRef{}, fmt.Errorf("private/publish: pour failed: %w", err)

	}

	return ref, nil
}
