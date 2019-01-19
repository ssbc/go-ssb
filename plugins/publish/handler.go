package publish

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
)

type handler struct {
	publish margaret.Log
	info    logging.Interface
}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if n := req.Method.String(); n != "publish" {
		req.CloseWithError(errors.Errorf("publish: bad request name: %s", n))
		return
	}
	if n := len(req.Args); n != 1 {
		req.CloseWithError(errors.Errorf("publish: bad request. expected 1 argument got %d", n))
		return
	}

	seq, err := h.publish.Append(req.Args[0])
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "publish: pour failed"))
		return
	}

	h.info.Log("published", seq.Seq())

	err = req.Return(ctx, fmt.Sprintf("published msg: %d", seq.Seq()))
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "publish: return failed"))
		return
	}
}

func (h handler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}
