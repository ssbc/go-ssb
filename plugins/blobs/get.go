// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"encoding/json"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	refs "go.mindeco.de/ssb-refs"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb/blobstore"

	"go.cryptoscope.co/ssb"
)

type getHandler struct {
	bs  ssb.BlobStore
	log logging.Interface
}

func (getHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h getHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	logger := log.With(h.log, "handler", "get")
	errLog := level.Error(logger)

	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "source"
	}

	var wantedRef *refs.BlobRef
	var maxSize uint = blobstore.DefaultMaxSize

	var justTheRef []refs.BlobRef
	if err := json.Unmarshal(req.RawArgs, &justTheRef); err != nil {
		var withSize []blobstore.GetWithSize
		if err := json.Unmarshal(req.RawArgs, &withSize); err != nil {
			req.Stream.CloseWithError(errors.Wrap(err, "bad request - invalid json"))
			return
		}
		if len(withSize) != 1 {
			req.Stream.CloseWithError(errors.New("bad request"))
			return
		}
		wantedRef = withSize[0].Key
		maxSize = withSize[0].Max
	} else {
		if len(justTheRef) != 1 {
			req.Stream.CloseWithError(errors.New("bad request"))
			return
		}
		wantedRef = &justTheRef[0]
	}

	sz, err := h.bs.Size(wantedRef)
	if err != nil {
		req.Stream.CloseWithError(errors.New("do not have blob"))
		return
	}

	if sz > 0 && uint(sz) > maxSize {
		req.Stream.CloseWithError(errors.New("blob larger than you wanted"))
		return
	}

	logger = log.With(logger, "blob", wantedRef.ShortRef())
	errLog = level.Error(logger)

	r, err := h.bs.Get(wantedRef)
	if err != nil {
		req.Stream.CloseWithError(errors.New("do not have blob"))
		return
	}

	w := muxrpc.NewSinkWriter(req.Stream)
	_, err = io.Copy(w, r)
	checkAndLog(errLog, errors.Wrap(err, "error sending blob"))

	err = w.Close()
	checkAndLog(errLog, errors.Wrap(err, "error closing blob output"))
	// if err == nil {
	// 	info.Log("event", "transmission successfull", "took", time.Since(start))
	// }
}
