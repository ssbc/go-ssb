package blobs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
)

type listHandler struct {
	bs sbot.BlobStore
}

func (listHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h listHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	log.Log("event", "onCall", "handler", "list", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "source"
	}

	err := luigi.Pump(ctx, req.Stream, h.bs.List())
	checkAndLog(errors.Wrap(err, "error listing blobs"))
}
