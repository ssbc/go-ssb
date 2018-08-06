package blobs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
)

type addHandler struct {
	bs sbot.BlobStore
}

func (addHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h addHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	log.Log("event", "onCall", "handler", "add", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "sink"
	}

	r := muxrpc.NewSourceReader(req.Stream)
	ref, err := h.bs.Put(r)
	checkAndLog(errors.Wrap(err, "error putting blob"))

	req.Return(ctx, ref)
}
