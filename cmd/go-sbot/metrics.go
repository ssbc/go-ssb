package main

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/cryptix/go/logging/countconn"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
)

var (
	SystemEvents  *prometheus.Counter
	SystemSummary *prometheus.Summary
	RepoStats     *prometheus.Gauge

	muxrpcSummary *prometheus.Summary
)

type latencyMuxH struct {
	root muxrpc.Handler
	sum  *prometheus.Summary
}

func (lm *latencyMuxH) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	start := time.Now()
	lm.root.HandleCall(ctx, req, EndpointWithLatency(lm.sum)(edp))
	lm.sum.With("method", req.Method.String(), "type", string(req.Type), "error", "undefined").Observe(time.Since(start).Seconds())

}

func (lm *latencyMuxH) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	start := time.Now()
	lm.root.HandleConnect(ctx, EndpointWithLatency(lm.sum)(edp))
	lm.sum.With("method", "none", "type", "connect", "error", "undefined").Observe(time.Since(start).Seconds())
}

func HandlerWithLatency(s *prometheus.Summary) muxrpc.HandlerWrapper {
	return func(root muxrpc.Handler) muxrpc.Handler {
		return &latencyMuxH{
			root: root,
			sum:  s,
		}
	}
}

func startDebug() {
	if debugAddr == "" {
		return
	}

	SystemEvents = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: "gossb",
		Subsystem: "events",
		Name:      "ssb_sysevents",
	}, []string{"event"})

	RepoStats = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Namespace: "gossb",
		Subsystem: "repo",
		Name:      "ssb_repostats",
	}, []string{"part"})

	muxrpcSummary = prometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "gossb",
		Subsystem: "muxrpc",
		Name:      "muxrpc_durrations_seconds",
	}, []string{"method", "type", "error"})

	SystemSummary = prometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "gossb",
		Subsystem: "sbot",
		Name:      "general_durrations",
	}, []string{"part"})

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Log("starting", "metrics", "addr", debugAddr)
		err := http.ListenAndServe(debugAddr, nil)
		checkAndLog(err)
	}()
}

type latencyWrapper struct {
	start time.Time
	root  muxrpc.Endpoint
	sum   *prometheus.Summary
}

func EndpointWithLatency(sum *prometheus.Summary) func(r muxrpc.Endpoint) muxrpc.Endpoint {
	return func(r muxrpc.Endpoint) muxrpc.Endpoint {
		var lw latencyWrapper
		lw.root = r
		lw.start = time.Now()
		lw.sum = sum
		return &lw
	}
}

func (lw *latencyWrapper) Async(ctx context.Context, tipe interface{}, method muxrpc.Method, args ...interface{}) (interface{}, error) {
	start := time.Now()
	val, err := lw.root.Async(ctx, tipe, method, args...)
	lw.sum.With("method", method.String(), "type", "async", "error", err.Error()).Observe(time.Since(start).Seconds())
	return val, err
}

func (lw *latencyWrapper) Source(ctx context.Context, tipe interface{}, method muxrpc.Method, args ...interface{}) (luigi.Source, error) {
	start := time.Now()
	rootSrc, err := lw.root.Source(ctx, tipe, method, args...)
	if err != nil {
		lw.sum.With("method", method.String(), "type", "source", "error", err.Error()).Observe(time.Since(start).Seconds())
		return nil, err
	}

	pSrc, pSink := luigi.NewPipe()
	go func() {
		var errStr = "nil"
		err := luigi.Pump(ctx, pSink, rootSrc)
		if err != nil {
			errStr = errors.Cause(err).Error()
		}
		pSink.Close()
		lw.sum.With("method", method.String(), "type", "source", "error", errStr).Observe(time.Since(start).Seconds())
	}()

	return pSrc, nil
}

func (lw *latencyWrapper) Sink(ctx context.Context, method muxrpc.Method, args ...interface{}) (luigi.Sink, error) {
	start := time.Now()
	rootSink, err := lw.root.Sink(ctx, method, args...)
	if err != nil {
		lw.sum.With("method", method.String(), "type", "sink", "error", err.Error()).Observe(time.Since(start).Seconds())
		return nil, err
	}

	pSrc, pSink := luigi.NewPipe()
	go func() {
		var errStr = "nil"
		err := luigi.Pump(ctx, rootSink, pSrc)
		if err != nil {
			errStr = errors.Cause(err).Error()
		}
		rootSink.Close()
		lw.sum.With("method", method.String(), "type", "sink", "error", errStr).Observe(time.Since(start).Seconds())
	}()

	return pSink, nil
}

func (lw *latencyWrapper) Duplex(ctx context.Context, tipe interface{}, method muxrpc.Method, args ...interface{}) (luigi.Source, luigi.Sink, error) {
	start := time.Now()
	rootSrc, rootSink, err := lw.root.Duplex(ctx, tipe, method, args...)
	if err != nil {
		lw.sum.With("method", method.String(), "type", "sink", "error", err.Error()).Observe(time.Since(start).Seconds())
		return nil, nil, err
	}

	roottoSrc, roottoSink := luigi.NewPipe()
	go func() {
		var errStr = "nil"
		err := luigi.Pump(ctx, rootSink, roottoSrc)
		if err != nil {
			errStr = errors.Cause(err).Error()
		}
		rootSink.Close()
		lw.sum.With("method", method.String(), "type", "duplex sink", "error", errStr).Observe(time.Since(start).Seconds())
	}()

	rootfromSrc, rootfromSink := luigi.NewPipe()
	go func() {
		var errStr = "nil"
		err := luigi.Pump(ctx, rootfromSink, rootSrc)
		if err != nil {
			errStr = errors.Cause(err).Error()
		}
		rootfromSink.Close()
		lw.sum.With("method", method.String(), "type", "duplex source", "error", errStr).Observe(time.Since(start).Seconds())
	}()

	return rootfromSrc, roottoSink, nil
}

// Assuming evrything goes through the above
func (lw *latencyWrapper) Do(ctx context.Context, req *muxrpc.Request) error {
	return lw.root.Do(ctx, req)
}

func (lw *latencyWrapper) Terminate() error {
	err := lw.root.Terminate()
	lw.sum.With("method", "terminate", "type", "close", "error", err.Error()).Observe(time.Since(lw.start).Seconds())
	return err
}

func (lw *latencyWrapper) Remote() net.Addr {
	return lw.root.Remote()
}

func (lw *latencyWrapper) Serve(ctx context.Context) error {
	srv, ok := lw.root.(muxrpc.Server)
	if !ok {
		return errors.Errorf("latencywrapper: server interface not implemented")
	}
	// this looses the wrapped endpoint again maybe?
	return srv.Serve(ctx)
}

type promCount struct {
	*countconn.Reader
	*countconn.Writer
	conn net.Conn
}

func promCountConn() netwrap.ConnWrapper {
	return func(c net.Conn) (net.Conn, error) {
		wrap := &promCount{
			conn: c,
		}
		wrap.Reader = countconn.NewReader(c)
		wrap.Writer = countconn.NewWriter(c)
		return wrap, nil
	}
}

func (c *promCount) Close() error {
	err := c.conn.Close()
	SystemEvents.With("event", "bytes.tx").Add(float64(c.Writer.N()))
	SystemEvents.With("event", "bytes.rx").Add(float64(c.Reader.N()))
	return err
}

func (c *promCount) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *promCount) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *promCount) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *promCount) SetReadDeadline(t time.Time) error  { return c.conn.SetReadDeadline(t) }
func (c *promCount) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }
