package gossip

import (
	"context"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
)

func New(repo sbot.Repo, node sbot.Node, promisc bool, log logging.Interface) sbot.Plugin {
	return plugin{
		&handler{
			Node:    node,
			Repo:    repo,
			Promisc: promisc,
			Info:    log,
		},
	}
}

type IgnoreConnectHandler struct{ muxrpc.Handler }

func (IgnoreConnectHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

func NewHist(repo sbot.Repo, node sbot.Node, log logging.Interface) sbot.Plugin {
	return histPlugin{
		&handler{
			Node:    node,
			Repo:    repo,
			Promisc: false,
			Info:    log,
		},
	}
}

type plugin struct {
	h *handler
}

func (plugin) Name() string { return "gossip" }

func (plugin) Method() muxrpc.Method {
	return muxrpc.Method{"gossip"}
}

func (p plugin) Handler() muxrpc.Handler {
	return p.h
}

type histPlugin struct {
	h *handler
}

func (hp histPlugin) Name() string { return "gossip" }

func (histPlugin) Method() muxrpc.Method {
	return muxrpc.Method{"createHistoryStream"}
}

func (hp histPlugin) Handler() muxrpc.Handler {
	return IgnoreConnectHandler{hp.h}
}
