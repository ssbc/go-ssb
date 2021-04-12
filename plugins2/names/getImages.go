// SPDX-License-Identifier: MIT

package names

import (
	"context"
	"fmt"

	"go.cryptoscope.co/muxrpc/v2"
	"go.mindeco.de/logging"
	refs "go.mindeco.de/ssb-refs"
)

type hImagesFor struct {
	as  aboutStore
	log logging.Interface
}

func (h hImagesFor) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {

	ref, err := parseFeedRefFromArgs(req)
	if err != nil {
		return nil, err
	}

	ai, err := h.as.CollectedFor(ref)
	if err != nil {
		return nil, fmt.Errorf("do not have about for: %s", ref.Ref())
	}

	if ai.Image.Chosen != "" {
		return ai.Image.Chosen, nil
	}

	// this is suboptimal, just got started but didnt finish
	// ideal would take into account who your friends are, not everyone you see
	var mostSet string
	var most = 0
	for v, cnt := range ai.Image.Prescribed {
		if most > cnt {
			most = cnt
			mostSet = v
		}
	}

	return mostSet, nil
}

func parseFeedRefFromArgs(req *muxrpc.Request) (refs.FeedRef, error) {
	args := req.Args()
	if len(args) != 1 {
		return refs.FeedRef{}, fmt.Errorf("not enough args")
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
		return refs.FeedRef{}, fmt.Errorf("error parsing feed reference: %w", err)
	}

	return ref, nil
}
