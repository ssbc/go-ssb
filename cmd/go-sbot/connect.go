package main

import (
	"context"
	"fmt"
	"net"

	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/sbot"
	"github.com/pkg/errors"
)

type connect struct {
	Node *sbot.Node
}

func (c *connect) HandleConnect(context.Context, muxrpc.Endpoint) {
	log.Log("event", "onConnect")
}

func (c *connect) HandleCall(ctx context.Context, req *muxrpc.Request) {
	log.Log("event", "onCall")
	if len(req.Args) != 1 {
		// TODO: use secretstream
		err := errors.New("usage: connect host:port")
		closeErr := req.Stream.CloseWithError(err)
		checkAndLog(errors.Wrapf(closeErr, "error closeing request. %s", err))
		return
	}

	addrString, ok := req.Args[0].(string)
	if !ok {
		log.Log("event", "bad request", "msg", fmt.Sprintf("expected argument to be %T, got %T\n", addrString, req.Args[0]))
		return
	}

	addr, err := net.ResolveTCPAddr("tcp", addrString)
	if err != nil {
		err = errors.Wrapf(err, "error resolving network address %q", addrString)
		log.Log("event", "call failed", "err", err)
		return
	}

	node := *(c.Node)
	err = node.Connect(ctx, addr)
	if err != nil {
		err = errors.Wrapf(err, "error connecting to %q", addr)
		log.Log("event", "call failed", "err", err)
	}
}
