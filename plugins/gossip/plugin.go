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

type HMACSecret *[32]byte

type HopCount int

type Promisc bool

func New(
	log logging.Interface,
	id *ssb.FeedRef,
	rootLog margaret.Log,
	userFeeds multilog.MultiLog,
	graphBuilder graph.Builder,
	opts ...interface{},
) *plugin {
	h := &handler{
		Id:           id,
		RootLog:      rootLog,
		UserFeeds:    userFeeds,
		GraphBuilder: graphBuilder,
		Info:         log,
	}

	for i, o := range opts {
		switch v := o.(type) {
		case *prometheus.Gauge:
			h.sysGauge = v
		case *prometheus.Counter:
			h.sysCtr = v
		case HopCount:
			h.hopCount = int(v)
		case HMACSecret:
			h.hmacSec = v
		case Promisc:
			h.promisc = bool(v)
		default:
			log.Log("warning", "unhandled option", "i", i, "type", fmt.Sprintf("%T", o))
		}
	}
	if h.hopCount == 0 {
		h.hopCount = 2
	}

	h.feedManager = NewFeedManager(
		h.RootLog,
		h.UserFeeds,
		h.Info,
		h.sysGauge,
		h.sysCtr,
	)

	return &plugin{h}
}

func NewHist(
	log logging.Interface,
	id *ssb.FeedRef,
	rootLog margaret.Log,
	userFeeds multilog.MultiLog,
	graphBuilder graph.Builder,
	opts ...interface{},
) histPlugin {
	h := &handler{
		Id:           id,
		RootLog:      rootLog,
		UserFeeds:    userFeeds,
		GraphBuilder: graphBuilder,
		Info:         log,
	}

	for i, o := range opts {
		switch v := o.(type) {
		case *prometheus.Gauge:
			h.sysGauge = v
		case *prometheus.Counter:
			h.sysCtr = v
		case Promisc:
			h.promisc = bool(v)
		case HopCount:
			h.hopCount = int(v)
		case HMACSecret:
			h.hmacSec = v
		default:
			log.Log("warning", "unhandled hist option", "i", i, "type", fmt.Sprintf("%T", o))
		}
	}

	if h.hopCount == 0 {
		h.hopCount = 2
	}

	h.feedManager = NewFeedManager(
		h.RootLog,
		h.UserFeeds,
		h.Info,
		h.sysGauge,
		h.sysCtr,
	)

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

func (hp histPlugin) Name() string { return "createHistoryStream" }

func (histPlugin) Method() muxrpc.Method {
	return muxrpc.Method{"createHistoryStream"}
}

type IgnoreConnectHandler struct{ muxrpc.Handler }

func (IgnoreConnectHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

func (hp histPlugin) Handler() muxrpc.Handler {
	return IgnoreConnectHandler{hp.h}
}
