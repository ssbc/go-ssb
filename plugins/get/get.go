// SPDX-License-Identifier: MIT

package get

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/shurcooL/go-goon"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	// "go.mindeco.de/encodedTime"
)

type plugin struct {
	h muxrpc.Handler
}

func (p plugin) Name() string {
	return "get"
}

func (p plugin) Method() muxrpc.Method {
	return muxrpc.Method{"get"}
}

func (p plugin) Handler() muxrpc.Handler {
	return p.h
}

func New(g ssb.Getter, rl margaret.Log) ssb.Plugin {
	return plugin{
		h: handler{g: g, rl: rl},
	}
}

type handler struct {
	g  ssb.Getter
	rl margaret.Log
}

func (h handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args()) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
		return
	}
	var (
		ref *ssb.MessageRef
		err error
	)
	switch v := req.Args()[0].(type) {
	case string:
		ref, err = ssb.ParseMessageRef(v)
	case map[string]interface{}:
		goon.Dump(v)
		refV, ok := v["key"]
		if !ok {
			seqV, ok := v["id"]
			if !ok {
				req.CloseWithError(errors.Errorf("invalid argument - missing 'id' in map"))
				return
			}
			seq, ok := seqV.(float64)
			if !ok {
				req.CloseWithError(errors.Errorf("invalid argument - missing 'id' in map"))
				return
			}
			sv, err := h.rl.Get(margaret.BaseSeq(seq))
			if err != nil {
				req.CloseWithError(errors.Wrap(err, "failed to get msg"))
				return
			}
			msg := sv.(ssb.Message)
			var kv ssb.KeyValueRaw
			kv.Key_ = msg.Key()
			kv.Value = *msg.ValueContent()
			// kv.Timestamp = encodedTime.Millisecs(time.Now())
			req.Return(ctx, kv)
			return
		}
		ref, err = ssb.ParseMessageRef(refV.(string))
	default:
		req.CloseWithError(errors.Errorf("invalid argument type %T", req.Args()[0]))
		return
	}

	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to parse arguments"))
		return
	}

	msg, err := h.g.Get(*ref)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to load message"))
		return
	}

	// var retMsg json.RawMessage
	// if msg.Author.Offchain {
	// 	var tmpMsg message.DeserializedMessage
	// 	tmpMsg.Previous = *msg.Previous
	// 	tmpMsg.Author = *msg.Author
	// 	tmpMsg.Sequence = msg.Sequence
	// 	// tmpMsg.Timestamp = msg. TODO: meh.. need to get the user-timestamp from the raw field
	// 	tmpMsg.Hash = msg.Key.Algo
	// 	tmpMsg.Content = msg.Offchain

	// 	retMsg, err = json.Marshal(tmpMsg)
	// 	if err != nil {
	// 		req.CloseWithError(errors.Wrap(err, "failed to re-wrap offchain message"))
	// 		return
	// 	}
	// } else {
	// retMsg = msg.Raw
	// }
	err = req.Return(ctx, msg.ValueContentJSON())
	if err != nil {
	}
	fmt.Println("get: failed? to return message:", err)

}
