// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

type sizeHandler struct {
	bs  ssb.BlobStore
	log logging.Interface
}

func (sizeHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h sizeHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	var blobs []*refs.BlobRef
	err := json.Unmarshal(req.RawArgs, &blobs)
	if err != nil {
		req.Stream.CloseWithError(fmt.Errorf("error parsing blob reference: %w", err))
		return
	}
	if len(blobs) != 1 {
		req.Stream.CloseWithError(fmt.Errorf("bad request - got %d arguments, expected 1", len(blobs)))
		return
	}
	sz, err := h.bs.Size(blobs[0])
	if err != nil {
		err = fmt.Errorf("error looking up blob: %w", err)
		err = req.Stream.CloseWithError(err)
		checkAndLog(h.log, err)
		return
	}

	err = req.Return(ctx, sz)
	if err != nil {
		checkAndLog(h.log, fmt.Errorf("error returning value: %w", err))
	}
}
