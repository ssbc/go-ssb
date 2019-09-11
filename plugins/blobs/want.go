package blobs

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
)

type wantHandler struct {
	wm  ssb.WantManager
	log logging.Interface
}

func (wantHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h wantHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	//h.log.Log("event", "onCall", "handler", "want", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	if len(req.Args) != 1 {
		// TODO: change from generic handlers to typed once (source, sink, async..)
		// async then would have to return a value or an error and not fall into this trap of not closing a stream
		req.Stream.CloseWithError(fmt.Errorf("bad request - wrong args (%d)", len(req.Args)))
		return
	}

	br, err := ssb.ParseBlobRef(req.Args[0].(string))
	if err != nil {
		err = errors.Wrap(err, "error parsing blob reference")
		checkAndLog(h.log, errors.Wrap(req.CloseWithError(err), "error returning error"))
		return
	}

	err = h.wm.Want(br)
	err = errors.Wrap(err, "error wanting blob reference")
	checkAndLog(h.log, errors.Wrap(req.Return(ctx, err), "error returning error"))
}
