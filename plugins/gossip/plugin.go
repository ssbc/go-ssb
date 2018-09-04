package gossip

import (
	"context"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/graph"
)

func New(log logging.Interface, id *sbot.FeedRef, rootLog margaret.Log, userFeeds multilog.MultiLog, graphBuilder graph.Builder, node sbot.Node, promisc bool) sbot.Plugin {
	return plugin{
		&handler{
			Node:         node,
			Id:           id,
			RootLog:      rootLog,
			UserFeeds:    userFeeds,
			GraphBuilder: graphBuilder,
			Info:         log,
			Promisc:      promisc,
			hanlderDone:  func() {},
		},
	}
}

func NewHist(log logging.Interface, id *sbot.FeedRef, rootLog margaret.Log, userFeeds multilog.MultiLog, graphBuilder graph.Builder, node sbot.Node) sbot.Plugin {
	return histPlugin{
		&handler{
			Node:         node,
			Id:           id,
			RootLog:      rootLog,
			UserFeeds:    userFeeds,
			GraphBuilder: graphBuilder,
			Info:         log,
			hanlderDone:  func() {},
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

type IgnoreConnectHandler struct{ muxrpc.Handler }

func (IgnoreConnectHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

func (hp histPlugin) Handler() muxrpc.Handler {
	return IgnoreConnectHandler{hp.h}
}
