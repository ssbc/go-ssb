package blobs

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
)

type hasHandler struct {
	bs sbot.BlobStore
}

func (hasHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h hasHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	log.Log("event", "onCall", "handler", "has", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	if len(req.Args) != 1 {
		return
	}

	ref, err := sbot.ParseRef(req.Args[0].(string))
	checkAndLog(errors.Wrap(err, "error parsing blob reference"))
	if err != nil {
		return
	}

	br, ok := ref.(*sbot.BlobRef)
	if !ok {
		err = errors.Errorf("expected blob reference, got %T", ref)
		checkAndLog(err)
		return
	}

	_, err = h.bs.Get(br)

	has := true

	if os.IsNotExist(errors.Cause(err)) {
		has = false
	} else if err != nil {
		err = errors.Wrap(err, "error looking up blob")
		err = req.Stream.CloseWithError(err)
		checkAndLog(err)
		return
	}

	err = req.Return(ctx, has)
	checkAndLog(errors.Wrap(err, "error returning value"))
}
