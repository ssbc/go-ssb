// SPDX-License-Identifier: MIT

// Package get is just a muxrpc wrapper around sbot.Get
package get

import (
	"context"
	"log"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/private"
	refs "go.mindeco.de/ssb-refs"
)

type plugin struct {
	h muxrpc.Handler
}

func (p plugin) Name() string            { return "get" }
func (p plugin) Method() muxrpc.Method   { return muxrpc.Method{"get"} }
func (p plugin) Handler() muxrpc.Handler { return p.h }

func New(g ssb.Getter, rxlog margaret.Log, unboxer *private.Manager) ssb.Plugin {
	return plugin{
		h: handler{
			get:     g,
			rxlog:   rxlog,
			unboxer: unboxer,
		},
	}
}

type handler struct {
	get     ssb.Getter
	rxlog   margaret.Log
	unboxer *private.Manager
}

func (h handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args()) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
		return
	}
	var (
		input  string
		parsed *refs.MessageRef
		err    error

		// decrypt a private message
		unboxPrivate bool
	)
	switch v := req.Args()[0].(type) {
	case string:
		input = v
	case map[string]interface{}:
		refV, ok := v["id"]
		if !ok {
			req.CloseWithError(errors.Errorf("invalid argument - missing 'id' in map"))
			return
		}
		input = refV.(string)

		if v, has := v["private"]; has {
			if yes, isBool := v.(bool); isBool {
				unboxPrivate = yes
			}
		}
	default:
		req.CloseWithError(errors.Errorf("invalid argument type %T", req.Args()[0]))
		return
	}

	parsed, err = refs.ParseMessageRef(input)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to parse arguments"))
		return
	}

	msg, err := h.get.Get(*parsed)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to load message"))
		return
	}
	var kv refs.KeyValueRaw
	kv.Key_ = msg.Key()
	kv.Value = *msg.ValueContent()

	if unboxPrivate {
		cleartext, err := h.unboxer.DecryptMessage(msg)
		if err == nil {
			kv.Value.Meta = make(map[string]interface{}, 1)
			kv.Value.Meta["private"] = true

			kv.Value.Content = cleartext
		} else if err != private.ErrNotBoxed {
			req.CloseWithError(errors.Wrap(err, "failed to decrypt message"))
			return
		}
	}

	err = req.Return(ctx, kv)
	if err != nil {
		log.Printf("get(%s): failed? to return message: %s", parsed.Ref(), err)
	}
}
