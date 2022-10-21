// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"fmt"

	"github.com/ssbc/go-muxrpc/v2"
	"go.mindeco.de/logging"

	"github.com/ssbc/go-ssb"
	refs "github.com/ssbc/go-ssb-refs"
)

type addHandler struct {
	self refs.FeedRef
	bs   ssb.BlobStore
	log  logging.Interface
}

func (addHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h addHandler) HandleSink(ctx context.Context, req *muxrpc.Request, src *muxrpc.ByteSource) error {
	requester, err := ssb.GetFeedRefFromAddr(req.RemoteAddr())
	if err != nil {
		return fmt.Errorf("unauthorized")
	}

	if !requester.Equal(h.self) {
		return fmt.Errorf("unauthorized")
	}

	r := muxrpc.NewSourceReader(src)
	ref, err := h.bs.Put(r)
	if err != nil {
		return fmt.Errorf("error putting blob: %w", err)
	}

	return req.Return(ctx, ref)
}
