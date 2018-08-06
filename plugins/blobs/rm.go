package blobs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
)

type rmHandler struct {
	bs sbot.BlobStore
}

func (rmHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h rmHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	log.Log("event", "onCall", "handler", "rm", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
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

	err = h.bs.Delete(br)
	if err != nil {
		err = req.Stream.CloseWithError(errors.New("do not have blob"))
	}

	checkAndLog(errors.Wrap(err, "error closing stream with error"))
}
