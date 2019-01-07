package gossip

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/metrics/prometheus"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
)

func New(log logging.Interface, id *ssb.FeedRef, rootLog margaret.Log, userFeeds multilog.MultiLog, graphBuilder graph.Builder, node ssb.Node, opts ...interface{}) ssb.Plugin {
	h := &handler{
		Node:         node,
		Id:           id,
		RootLog:      rootLog,
		UserFeeds:    userFeeds,
		GraphBuilder: graphBuilder,
		Info:         log,
		hanlderDone:  func() {},
	}
	for i, o := range opts {
		switch v := o.(type) {
		case *prometheus.Gauge:
			h.sysGauge = v
		case *prometheus.Counter:
			h.sysCtr = v
		default:
			log.Log("warning", "unhandled option", "i", i, "type", fmt.Sprintf("%T", o))
		}
	}
	return &plugin{h}
}

func NewHist(log logging.Interface, id *ssb.FeedRef, rootLog margaret.Log, userFeeds multilog.MultiLog, graphBuilder graph.Builder, node ssb.Node, opts ...interface{}) ssb.Plugin {
	h := &handler{
		Node:         node,
		Id:           id,
		RootLog:      rootLog,
		UserFeeds:    userFeeds,
		GraphBuilder: graphBuilder,
		Info:         log,
		hanlderDone:  func() {},
	}
	for i, o := range opts {
		switch v := o.(type) {
		case *prometheus.Gauge:
			h.sysGauge = v
		case *prometheus.Counter:
			h.sysCtr = v
		default:
			log.Log("warning", "unhandled hist option", "i", i, "type", fmt.Sprintf("%T", o))
		}
	}
	return histPlugin{h}
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
