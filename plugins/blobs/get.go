package blobs

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
)

type getHandler struct {
	bs sbot.BlobStore
}

func (getHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h getHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	log.Log("event", "onCall", "handler", "get", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "source"
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

	r, err := h.bs.Get(br)
	if err != nil {
		err = req.Stream.CloseWithError(errors.New("do not have blob"))
		checkAndLog(errors.Wrap(err, "error closing stream with error"))
		return
	}

	w := muxrpc.NewSinkWriter(req.Stream)
	_, err = io.Copy(w, r)
	checkAndLog(errors.Wrap(err, "error sending blob"))

	checkAndLog(errors.Wrap(req.Stream.Close(), "failed to close get stream"))
}
