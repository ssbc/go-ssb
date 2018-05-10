package main

import (
	"context"
	"log"
	"net"

	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/sbot"
	"github.com/pkg/errors"
)

type connect struct {
	Node *sbot.Node
}

func (c *connect) HandleConnect(context.Context, muxrpc.Endpoint) {
	log.Println("onConnect")
}
func (c *connect) HandleCall(ctx context.Context, req *muxrpc.Request) {
	log.Println("onCall")
	if len(req.Args) != 1 {
		// TODO: use secretstream
		err := errors.New("usage: connect host:port")
		closeErr := req.Stream.CloseWithError(err)
		if closeErr != nil {
			log.Println("error closing request with error:", closeErr)
			log.Println("close cause:", err)
		}

		return
	}

	addrString, ok := req.Args[0].(string)
	if !ok {
		log.Printf("expected argument to be %T, got %T\n", addrString, req.Args[0])
		return
	}

	addr, err := net.ResolveTCPAddr("tcp", addrString)
	if err != nil {
		log.Printf("error resolving network address %q: %s\n", addrString, err)
		return
	}

	node := *(c.Node)
	err = node.Connect(ctx, addr)
	if err != nil {
		log.Printf("error connecting to %q: %s\n", addr, err)
	}
}
