package main

import (
	"context"
	"fmt"

	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
)

type whoAmI struct {
	I sbot.FeedRef
}

func (whoAmI) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	//srv := edp.(muxrpc.Server)
	//log.Log("event", "onConnect", "handler", "whoami", "addr", srv.Remote())
}

func (wami whoAmI) HandleCall(ctx context.Context, req *muxrpc.Request) {
	log.Log("event", "onCall", "handler", "connect", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
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
