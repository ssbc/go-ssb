// SPDX-License-Identifier: MIT

package control

import (
	"context"
	"os"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb/internal/muxmux"
	multiserver "go.mindeco.de/ssb-multiserver"

	"go.cryptoscope.co/ssb"
)

type handler struct {
	node ssb.Network
	info logging.Interface
}

func New(i logging.Interface, n ssb.Network) muxrpc.Handler {

	h := &handler{
		info: i,
		node: n,
	}

	mux := muxmux.New(i)

	mux.RegisterAsync(muxrpc.Method{"ctrl", "connect"}, muxmux.AsyncFunc(h.connect))
	mux.RegisterAsync(muxrpc.Method{"ctrl", "disconnect"}, muxmux.AsyncFunc(h.disconnect))

	return &mux
}

func (h *handler) disconnect(ctx context.Context, r *muxrpc.Request) (interface{}, error) {
	h.node.GetConnTracker().CloseAll()
	return "disconencted", nil
}

func (h *handler) check(err error) {
	if err != nil && errors.Cause(err) != os.ErrClosed {
		h.info.Log("error", err)
	}
}

func (h *handler) connect(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	if len(req.Args()) != 1 {
		h.info.Log("error", "usage", "args", req.Args, "method", req.Method)
		return nil, errors.New("usage: ctrl.connect host:port:key")

	}
	dest, ok := req.Args()[0].(string)
	if !ok {
		return nil, errors.Errorf("ctrl.connect call: expected argument to be string, got %T", req.Args()[0])
	}
	msaddr, err := multiserver.ParseNetAddress([]byte(dest))
	if err != nil {
		return nil, errors.Wrapf(err, "gossip.connect call: failed to parse input: %s", dest)
	}

	wrappedAddr := netwrap.WrapAddr(&msaddr.Addr, secretstream.Addr{PubKey: msaddr.Ref.PubKey()})
	level.Info(h.info).Log("event", "doing gossip.connect", "remote", msaddr.Ref.ShortRef())
	// TODO: add context to tracker to cancel connections
	err = h.node.Connect(context.Background(), wrappedAddr)
	return nil, errors.Wrapf(err, "gossip.connect call: error connecting to %q", msaddr.Addr)
}
