package blobs

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
)

type hasHandler struct {
	bs  sbot.BlobStore
	log logging.Interface
}

func (hasHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h hasHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	h.log.Log("event", "onCall", "handler", "has", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	if len(req.Args) != 1 {
		return
	}

	ref, err := sbot.ParseBlobRef(req.Args[0].(string))
	checkAndLog(h.log, errors.Wrap(err, "error parsing blob reference"))
	if err != nil {
		return
	}

	_, err = h.bs.Get(ref)

	has := true

	if err == blobstore.ErrNoSuchBlob {
		has = false
	} else if err != nil {
		err = errors.Wrap(err, "error looking up blob")
		err = req.Stream.CloseWithError(err)
		checkAndLog(h.log, err)
		return
	}

	err = req.Return(ctx, has)
	checkAndLog(h.log, errors.Wrap(err, "error returning value"))
}
