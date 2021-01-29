// SPDX-License-Identifier: MIT

package names

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc/v2"
	refs "go.mindeco.de/ssb-refs"
)

type hImagesFor struct {
	as  aboutStore
	log logging.Interface
}

func (hImagesFor) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h hImagesFor) HandleCall(ctx context.Context, req *muxrpc.Request) {
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	ref, err := parseFeedRefFromArgs(req)
	if err != nil {
		checkAndLog(h.log, err)
		return
	}

	ai, err := h.as.CollectedFor(ref)
	if err != nil {
		err = req.Stream.CloseWithError(fmt.Errorf("do not have about for: %s", ref.Ref()))
		checkAndLog(h.log, fmt.Errorf("error closing stream with error: %w", err))
		return
	}
	if ai.Image.Chosen != "" {
		err = req.Return(ctx, ai.Image.Chosen)
		checkAndLog(h.log, fmt.Errorf("error returning chosen value: %w", err))
		return
	}
	var hottest string
	var most = 0
	for v, cnt := range ai.Image.Prescribed {
		if most > cnt {
			most = cnt
			hottest = v
		}
	}
	err = req.Return(ctx, hottest)
	if err != nil {
		checkAndLog(h.log, fmt.Errorf("error returning chosen value: %w", err))
	}
	return
}

func checkAndLog(log logging.Interface, err error) {
	if err != nil {
		log.Log("handlerErr", err)
	}
}

func parseFeedRefFromArgs(req *muxrpc.Request) (*refs.FeedRef, error) {
	args := req.Args()
	if len(args) != 1 {
		return nil, fmt.Errorf("not enough args")
	}

	var refStr string
	switch arg := args[0].(type) {
	case string:
		refStr = arg
	case map[string]interface{}:
		refStr, _ = arg["id"].(string)
	}

	ref, err := refs.ParseFeedRef(refStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing feed reference: %w", err)
	}

	return ref, nil
}
