package groups

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb/private"
	refs "go.mindeco.de/ssb-refs"
)

type create struct {
	log log.Logger

	groups *private.Manager
}

func (h create) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	var args []struct {
		Name string
	}
	if err := json.Unmarshal(req.RawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid argument on groups.create call: %w", err)
	}

	if len(args) != 1 {
		return nil, fmt.Errorf("expected one arg {name}")
	}
	a := args[0]

	cloaked, root, err := h.groups.Create(a.Name)
	if err != nil {
		return nil, err
	}

	level.Info(h.log).Log("event", "group created", "cloaked", cloaked.Ref())

	return struct {
		Group *refs.MessageRef `json:"group_id"`
		Root  *refs.MessageRef `json:"root"`
	}{cloaked, root}, err
}

type publishTo struct {
	log log.Logger

	groups *private.Manager
}

func (h publishTo) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	var args []json.RawMessage
	if err := json.Unmarshal(req.RawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid argument on publishTo call: %w", err)
	}
	if len(args) != 2 {
		return nil, fmt.Errorf("expected two args [groupID, content]")
	}

	var groupID refs.MessageRef
	err := json.Unmarshal(args[0], &groupID)
	if err != nil {
		return nil, fmt.Errorf("groupID needs to be a valid message ref: %w", err)
	}

	if groupID.Algo != refs.RefAlgoCloakedGroup {
		return nil, fmt.Errorf("groupID needs to be a cloaked message ref, not %s", groupID.Algo)
	}

	newMsg, err := h.groups.PublishTo(&groupID, args[1])
	if err != nil {
		return nil, fmt.Errorf("failed to publish message to group")
	}

	return newMsg.Ref(), nil
}

/*
type isBlockingH struct {
	self refs.FeedRef

	log log.Logger

	builder graph.Builder
}

func (h isBlockingH) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	var args []sourceDestArg
	if err := json.Unmarshal(req.RawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid argument on isBlocking call: %w", err)
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("expected one arg {source, dest}")
	}
	a := args[0]

	g, err := h.builder.Build()
	if err != nil {
		return nil, err
	}

	return g.Blocks(&a.Source, &a.Dest), nil
}

type plotSVGHandler struct {
	self refs.FeedRef

	log log.Logger

	builder graph.Builder
}

func (h plotSVGHandler) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	g, err := h.builder.Build()
	if err != nil {
		return nil, err
	}

	fname, err := ioutil.TempFile("", "graph-*.svg")
	if err != nil {
		return nil, err
	}

	err = g.RenderSVG(fname)
	if err != nil {
		fname.Close()
		os.Remove(fname.Name())
		return nil, err
	}

	return fname.Name(), fname.Close()
}
*/
