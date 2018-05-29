package main

import (
	"context"

	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/sbot"
	"github.com/pkg/errors"
)

func (edp *whoAmIEndpoint) WhoAmI(ctx context.Context) ([]byte, error) {
	v, err := edp.Async(ctx, []byte{}, []string{"whoami"})
	if err != nil {
		return nil, errors.Wrap(err, "error doing async call")
	}

	key, ok := v.([]byte)
	if !ok {
		return nil, errors.Errorf("expected type %T, got %T", key, v)
	}

	return key, nil
}

type whoAmI struct {
	I sbot.FeedRef
}

func (whoAmI) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	// TODO: retreive key from endpoint
	log.Log("event", "incomming connect")

	/* dont call back
	remote := &whoAmIEndpoint{edp}
	key, err := remote.WhoAmI(ctx)
	checkAndLog(errors.Wrap(err, "error calling whoami on remote"))

	if err == nil {
		log.Log("event", "called whoami", "key", string(key))
	}
	*/
}

func (wami whoAmI) HandleCall(ctx context.Context, req *muxrpc.Request) {
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}
	type ret struct {
		ID string `json:"id"`
	}
	err := req.Return(ctx, ret{wami.I.Ref()})
	checkAndLog(err)
}
