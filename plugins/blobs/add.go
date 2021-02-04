// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"

	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
)

type addHandler struct {
	bs  ssb.BlobStore
	log logging.Interface
}

func (addHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h addHandler) HandleAsync(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: push manifest check into muxrpc

	src, err := req.ResponseSource()
	if err != nil {
		err = fmt.Errorf("add: couldn't get source: %w", err)
		checkAndLog(h.log, err)
		req.CloseWithError(err)
		return
	}

	r := muxrpc.NewSourceReader(src)
	ref, err := h.bs.Put(r)
	if err != nil {
		err = fmt.Errorf("error putting blob: %w", err)
		checkAndLog(h.log, err)
		req.CloseWithError(err)
		return
	}

	req.Return(ctx, ref)
}
