package control

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
)

type handler struct {
	node ssb.Node
	info logging.Interface

	publish margaret.Log
}

func New(i logging.Interface, n ssb.Node, p margaret.Log) muxrpc.Handler {
	return &handler{
		publish: p,
		info:    i,
		node:    n,
	}
}

func (h *handler) check(err error) {
	if err != nil {
		h.info.Log("error", err)
	}
}

func (h *handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
}

func (h *handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if req.Type == "" {
		req.Type = "async"
	}

	var closed bool
	checkAndClose := func(err error) {
		h.check(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			h.check(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	defer func() {
		if !closed {
			h.check(errors.Wrapf(req.Stream.Close(), "gossip: error closing call: %s", req.Method))
		}
	}()

	switch req.Method.String() {

	case "ctrl.connect":
		if len(req.Args) != 1 {
			// TODO: use secretstream
			h.info.Log("error", "usage", "args", req.Args, "method", req.Method)
			checkAndClose(errors.New("usage: ctrl.connect host:port:key"))
			return
		}
		destString, ok := req.Args[0].(string)
		if !ok {
			err := errors.Errorf("ctrl.connect call: expected argument to be string, got %T", req.Args[0])
			checkAndClose(err)
			return
		}
		if err := h.connect(ctx, destString); err != nil {
			checkAndClose(errors.Wrap(err, "ctrl.connect failed."))
			return
		}
		closed = true
		h.check(req.Return(ctx, "connected"))

	// TODO: our plugins have to share one root namespace
	case "ctrl.publish":
		if n := len(req.Args); n != 1 {
			req.CloseWithError(errors.Errorf("publish: bad request. expected 1 argument got %d", n))
			return
		}

		seq, err := h.publish.Append(req.Args[0])
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "publish: pour failed"))
			return
		}
		h.info.Log("published", seq.Seq())
		checkAndClose(req.Return(ctx, fmt.Sprintf("published msg: %d", seq.Seq())))
	default:
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
	}
}

func (h *handler) connect(ctx context.Context, dest string) error {
	splitted := strings.Split(dest, ":")
	if n := len(splitted); n != 3 {
		return errors.Errorf("gossip.connect: bad request. expected 3 parts, got %d", n)
	}

	addr, err := net.ResolveTCPAddr("tcp", strings.Join(splitted[:2], ":"))
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: error resolving network address %q", splitted[:2])
	}

	ref, err := ssb.ParseRef(splitted[2])
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: failed to parse FeedRef %s", splitted[2])
	}

	remoteFeed, ok := ref.(*ssb.FeedRef)
	if !ok {
		return errors.Errorf("gossip.connect: expected FeedRef got %T", ref)
	}

	wrappedAddr := netwrap.WrapAddr(addr, secretstream.Addr{PubKey: remoteFeed.ID})
	h.info.Log("event", "doing gossip.connect", "remote", wrappedAddr.String())
	err = h.node.Connect(ctx, wrappedAddr)
	return errors.Wrapf(err, "gossip.connect call: error connecting to %q", addr)
}
