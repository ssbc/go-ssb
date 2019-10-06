// SPDX-License-Identifier: MIT

package names

import (
	"context"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
)

type hGetSignifier struct {
	as  aboutStore
	log logging.Interface
}

func (hGetSignifier) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h hGetSignifier) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	ref, err := parseFeedRefFromArgs(req)
	if err != nil {
		checkAndLog(h.log, err)
		req.CloseWithError(err)
		return
	}

	ai, err := h.as.CollectedFor(ref)
	if err != nil {
		err = req.Stream.CloseWithError(errors.Wrapf(err, "do not have about for: %s", ref.Ref()))
		checkAndLog(h.log, errors.Wrap(err, "error closing stream with error"))
		return
	}
	var name = ai.Name.Chosen
	if name == "" {
		for n := range ai.Name.Prescribed { // pick random name
			name = n
			break
		}
		if name == "" {
			name = ref.Ref()
		}
	}

	err = req.Return(ctx, name)
	checkAndLog(h.log, errors.Wrap(err, "error returning all values"))
	return
}
