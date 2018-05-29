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
		if len(req.Args) != 1 {
			// TODO: use secretstream
			log.Log("error", "usage", "args", req.Args, "method", req.Method)
			checkAndClose(errors.New("usage: gossip.connect host:port:key"))
			return
		}

		destString, ok := req.Args[0].(string)
		if !ok {
			err := errors.Errorf("gossip.connect call: expected argument to be string, got %T", req.Args[0])
			checkAndClose(err)
			return
		}

		if err := c.connect(ctx, destString); err != nil {
			err := errors.Wrap(err, "gossip.connect failed.")
			checkAndClose(err)
			return
		}
	default:
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
	}
}

func (g *gossip) connect(ctx context.Context, dest string) error {
	splitted := strings.Split(dest, ":")
	if n := len(splitted); n != 3 {
		return errors.Errorf("gossip.connect: bad request. expected 3 parts, got %d", n)
	}

	addr, err := net.ResolveTCPAddr("tcp", strings.Join(splitted[:2], ":"))
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: error resolving network address %q", splitted[:2])
	}

	ref, err := sbot.ParseRef(splitted[2])
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: failed to parse FeedRef %s", splitted[2])
	}

	remoteFeed, ok := ref.(*sbot.FeedRef)
	if !ok {
		return errors.Errorf("gossip.connect: expected FeedRef got %T", ref)
	}

	wrappedAddr := netwrap.WrapAddr(addr, secretstream.Addr{PubKey: remoteFeed.ID})
	err = g.Node.Connect(ctx, wrappedAddr)
	return errors.Wrapf(err, "gossip.connect call: error connecting to %q", addr)
}
