// SPDX-License-Identifier: MIT

package names

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc/v2"
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
		err = req.Stream.CloseWithError(fmt.Errorf("do not have about for: %s: %w", ref.Ref(), err))
		checkAndLog(h.log, fmt.Errorf("error closing stream with error: %w", err))
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
	if err != nil {
		checkAndLog(h.log, fmt.Errorf("error returning all values: %w", err))
	}
	return
}
