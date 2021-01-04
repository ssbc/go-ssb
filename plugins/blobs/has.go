// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	refs "go.mindeco.de/ssb-refs"
)

type hasHandler struct {
	bs  ssb.BlobStore
	log logging.Interface
}

func (hasHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h hasHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	if len(req.Args()) != 1 {
		// TODO: change from generic handlers to typed once (source, sink, async..)
		// async then would have to return a value or an error and not fall into this trap of not closing a stream
		req.Stream.CloseWithError(fmt.Errorf("bad request - wrong args"))
		return
	}

	switch v := req.Args()[0].(type) {
	case string:

		ref, err := refs.ParseBlobRef(v)
		if err != nil {
			req.Stream.CloseWithError(fmt.Errorf("error parsing blob reference: %w", err))
			return
		}

		_, err = h.bs.Get(ref)

		has := true

		if err == blobstore.ErrNoSuchBlob {
			has = false
		} else if err != nil {
			err = fmt.Errorf("error looking up blob: %w", err)
			err = req.Stream.CloseWithError(err)
			checkAndLog(h.log, err)
			return
		}

		err = req.Return(ctx, has)
		if err != nil {
			checkAndLog(h.log, fmt.Errorf("error returning value: %w", err))
		}

	case []interface{}:
		var has = make([]bool, len(v))

		for k, blobRef := range v {

			blobStr, ok := blobRef.(string)
			if !ok {
				req.Stream.CloseWithError(fmt.Errorf("bad request - unhandled type"))
				return
			}
			ref, err := refs.ParseBlobRef(blobStr)
			if err != nil {
				checkAndLog(h.log, fmt.Errorf("error parsing blob reference: %w", err))
				return
			}

			_, err = h.bs.Get(ref)

			has[k] = true

			if err == blobstore.ErrNoSuchBlob {
				has[k] = false
			} else if err != nil {
				err = fmt.Errorf("error looking up blob: %w", err)
				err = req.Stream.CloseWithError(err)
				checkAndLog(h.log, err)
				return
			}

		}

		err := req.Return(ctx, has)
		if err != nil {
			checkAndLog(h.log, fmt.Errorf("error returning value: %w", err))
		}

	default:
		req.Stream.CloseWithError(fmt.Errorf("bad request - unhandled type"))
		return
	}

}
