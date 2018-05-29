package main

import (
	"context"
	"fmt"
	"net"
	"strings"

	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/netwrap"
	"cryptoscope.co/go/secretstream"
	"github.com/pkg/errors"

	"cryptoscope.co/go/sbot"
)

type gossip struct {
	Node sbot.Node
}

func (c *gossip) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	srv := e.(muxrpc.Server)
	log.Log("event", "onConnect", "handler", "gossip", "addr", srv.Remote())
}

func (c *gossip) HandleCall(ctx context.Context, req *muxrpc.Request) {
	log.Log("event", "onCall", "handler", "gossip", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)

	checkAndClose := func(err error) {
		checkAndLog(err)
		if err != nil {
			closeErr := req.Stream.CloseWithError(err)
			checkAndLog(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	switch req.Method.String() {
	case "gossip.connect":
	default:
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
	}

	if len(req.Args) != 1 {
		// TODO: use secretstream
		log.Log("error", "usage", "args", req.Args, "method", req.Method)
		checkAndClose(errors.New("usage: gossip.connect host:port:key"))
		return
	}

	destString, ok := req.Args[0].(string)
	if !ok {
		err := errors.Errorf("gossip.connect call: expected argument to be string, got %T\n", req.Args[0])
		checkAndClose(err)
		return
	}

	splitted := strings.Split(destString, ":")
	if n := len(splitted); n != 3 {
		checkAndClose(errors.Errorf("gossip.connect: bad request. expected 3 parts, got %d", n))
		return
	}

	addr, err := net.ResolveTCPAddr("tcp", strings.Join(splitted[:2], ":"))
	if err != nil {
		err = errors.Wrapf(err, "gossip.connect call: error resolving network address %q", splitted[:2])
		checkAndClose(err)
		return
	}

	ref, err := sbot.ParseRef(splitted[2])
	if err != nil {
		err = errors.Wrapf(err, "gossip.connect call: failed to parse FeedRef %s", splitted[2])
		checkAndClose(err)
		return
	}

	remoteFeed, ok := ref.(*sbot.FeedRef)
	if !ok {
		checkAndClose(errors.Errorf("gossip.connect: expected FeedRef got %T", ref))
		return
	}

	wrappedAddr := netwrap.WrapAddr(addr, secretstream.Addr{PubKey: remoteFeed.ID})
	err = c.Node.Connect(ctx, wrappedAddr)
	if err != nil {
		err = errors.Wrapf(err, "gossip.connect call: error connecting to %q", addr)
		checkAndClose(err)
	}
}
