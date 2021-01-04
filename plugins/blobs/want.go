// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc/v2"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
)

type wantHandler struct {
	wm  ssb.WantManager
	log logging.Interface
}

func (wantHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h wantHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	//h.log.Log("event", "onCall", "handler", "want", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	args := req.Args()
	if len(args) != 1 {
		// TODO: change from generic handlers to typed once (source, sink, async..)
		// async then would have to return a value or an error and not fall into this trap of not closing a stream
		req.Stream.CloseWithError(fmt.Errorf("bad request - wrong args (%d)", len(args)))
		return
	}

	br, err := refs.ParseBlobRef(args[0].(string))
	if err != nil {
		err = fmt.Errorf("error parsing blob reference: %w", err)
		checkAndLog(h.log, fmt.Errorf("error returning error: %w", req.CloseWithError(err)))
		return
	}

	err = h.wm.Want(br)
	if err != nil {
		err = fmt.Errorf("error wanting blob reference: %w", err)
		checkAndLog(h.log, fmt.Errorf("error returning error: %w", req.Return(ctx, err)))
	}
}
