package gossip

import (
	"github.com/cryptix/go/logging"

	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
)

func New(
	repo sbot.Repo,
	node sbot.Node,
	promisc bool,
	log logging.Interface,
) sbot.Plugin {
	return plugin{
		&handler{
			Node:    node,
			Repo:    repo,
			Promisc: promisc,
			Info:    log,
		},
	}
}

func NewHist(
	repo sbot.Repo,
	node sbot.Node,
	promisc bool,
	log logging.Interface,
) sbot.Plugin {
	return histPlugin{
		plugin{
			&handler{
				Node:    node,
				Repo:    repo,
				Promisc: promisc,
				Info:    log,
			},
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
	sbot.Plugin
}

func (histPlugin) Method() muxrpc.Method {
	return muxrpc.Method{"createHistoryStream"}
}
