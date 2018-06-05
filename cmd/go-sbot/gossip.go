package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/netwrap"
	"cryptoscope.co/go/sbot"
	"cryptoscope.co/go/secretstream"
	"cryptoscope.co/go/ssb"
	"github.com/pkg/errors"
)

type gossip struct {
	Node sbot.Node
}

func (c *gossip) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	srv := e.(muxrpc.Server)
	log.Log("event", "onConnect", "handler", "gossip", "addr", srv.Remote())
	shsID := netwrap.GetAddr(srv.Remote(), "shs-bs").(secretstream.Addr)
	ref, err := sbot.ParseRef(shsID.String())
	if err != nil {
		log.Log("handleConnect", "sbot.ParseRef", "err", err)
		return
	}

	var q = ssb.CreateHistArgs{false, false, ref.Ref(), 0}
	source, err := e.Source(ctx, ssb.RawSignedMessage{}, []string{"createHistoryStream"}, q)
	if err != nil {
		log.Log("handleConnect", "createHistoryStream", "err", err)
		return
	}
	i := 0

	for {
		start := time.Now()
		v, err := source.Next(ctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			log.Log("handleConnect", "createHistoryStream", "i", i, "err", err)
			break
		}

		rmsg := v.(ssb.RawSignedMessage)

		ref, err := ssb.Verify(rmsg.RawMessage)
		if err != nil {
			err = errors.Wrap(err, "simple Encode failed")
			log.Log("handleConnect", "createHistoryStream", "i", i, "err", err)
			break
		}
		log.Log("event", "verfied", "hist", i, "ref", ref.Ref(), "took", time.Since(start))
		i++
	}
}

func (c *gossip) HandleCall(ctx context.Context, req *muxrpc.Request) {
	log.Log("event", "onCall", "handler", "gossip", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)

	var closed bool
	checkAndClose := func(err error) {
		checkAndLog(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			checkAndLog(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	defer func() {
		if !closed {
			checkAndLog(errors.Wrapf(req.Stream.Close(), "gossip: error closing call: %s", req.Method))
		}
	}()

	switch req.Method.String() {
	case "gossip.ping":
		if err := c.ping(ctx, req.Args); err != nil {
			checkAndClose(errors.Wrap(err, "gossip.ping failed."))
			return
		}

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
			checkAndClose(errors.Wrap(err, "gossip.connect failed."))
			return
		}
	default:
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
	}
}

func (g *gossip) ping(ctx context.Context, args []interface{}) error {
	log.Log("event", "ping", "args", fmt.Sprintf("%v", args))
	return errors.New("TODO")
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
