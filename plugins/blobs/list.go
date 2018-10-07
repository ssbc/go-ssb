package blobs

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
)

type listHandler struct {
	bs  ssb.BlobStore
	log logging.Interface
}

func (listHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h listHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	h.log.Log("event", "onCall", "handler", "list", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "source"
	}

	err := luigi.Pump(ctx, req.Stream, h.bs.List())
	checkAndLog(h.log, errors.Wrap(err, "error listing blobs"))
}
