package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/muxrpc/v2/typemux"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

// TunnelPlugin returns a muxrpc plugin that is able to handle incoming tunnel.connect requests
func (n *node) TunnelPlugin() ssb.Plugin {
	tunnelLogger := kitlog.With(n.log, "unit", "tunnel")
	rootHdlr := typemux.New(tunnelLogger)

	rootHdlr.RegisterAsync(muxrpc.Method{"tunnel", "isRoom"}, isRoomhandler{})
	rootHdlr.RegisterDuplex(muxrpc.Method{"tunnel", "connect"}, connectHandler{
		network: n,
		logger:  tunnelLogger,
	})

	return plugin{
		h: handleNewConnection{
			Handler: &rootHdlr,
			logger:  tunnelLogger,
		},
	}
}

// muxrpc shim
type plugin struct{ h muxrpc.Handler }

func (plugin) Name() string              { return "tunnel" }
func (plugin) Method() muxrpc.Method     { return muxrpc.Method{"tunnel"} }
func (p plugin) Handler() muxrpc.Handler { return p.h }

// tunnel.isRoom should return true for a tunnel server and false for clients
type isRoomhandler struct{}

func (h isRoomhandler) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	return false, nil
}

type connectHandler struct {
	network *node

	logger kitlog.Logger
}

func (h connectHandler) HandleDuplex(ctx context.Context, req *muxrpc.Request, peerSrc *muxrpc.ByteSource, peerSnk *muxrpc.ByteSink) error {
	portal, err := ssb.GetFeedRefFromAddr(req.Endpoint().Remote())
	if err != nil {
		return err
	}

	portalLogger := kitlog.With(h.logger, "portal", portal.ShortRef())
	level.Info(portalLogger).Log("event", "incomming tunnel.connect", "args", string(req.RawArgs))

	// wrap muxrpc duplex into a net.Conn like thing
	var tc tunnelConn
	tc.Reader = muxrpc.NewSourceReader(peerSrc)
	tc.WriteCloser = muxrpc.NewSinkWriter(peerSnk)
	tc.local = h.network.opts.ListenAddr

	tc.remote = tunnelHost{
		Host: *portal,
	}

	authWrapper := h.network.secretServer.ConnWrapper()

	conn, err := authWrapper(tc)
	if err != nil {
		level.Warn(portalLogger).Log("event", "tunnel.connect failed to authenticate", "err", err)
		return err
	}

	origin, err := ssb.GetFeedRefFromAddr(conn.RemoteAddr())
	if err != nil {
		level.Warn(portalLogger).Log("event", "failed to get feed for remote tunnel", "err", err)
		return err
	}

	level.Info(portalLogger).Log("event", "tunnel.connect established", "origin", origin.ShortRef())

	// start serving the connection
	go h.network.handleConnection(ctx, conn, true)

	return nil
}

// tunnelHost is a net.Addr for a tunnel server
type tunnelHost struct {
	Host refs.FeedRef
}

func (ta tunnelHost) Network() string {
	return "ssb-tunnel"
}

func (ta tunnelHost) String() string {
	return ta.Network() + ":" + ta.Host.Ref()
}

var _ net.Addr = tunnelHost{}

// tunnelConn wrapps a reader and writer with two hardcoded net address to behave like a net.Conn
type tunnelConn struct {
	local, remote net.Addr

	io.Reader
	io.WriteCloser
}

var _ net.Conn = tunnelConn{}

func (c tunnelConn) LocalAddr() net.Addr  { return c.local }
func (c tunnelConn) RemoteAddr() net.Addr { return c.remote }

func (c tunnelConn) SetDeadline(t time.Time) error {
	return fmt.Errorf("Deadlines unsupported")
}
func (c tunnelConn) SetReadDeadline(t time.Time) error {
	return fmt.Errorf("Read Deadlines unsupported")
}
func (c tunnelConn) SetWriteDeadline(t time.Time) error {
	return fmt.Errorf("Write Deadlines unsupported")
}

// handleNewConnection wrapps a muxrpc.Handler to do some stuff with new connections
type handleNewConnection struct {
	muxrpc.Handler

	logger kitlog.Logger
}

// HandleConnect checks if a new connection is a room (via tunnel.isRoom) and if it is,
// it opens and outputs tunnel.endpoints updates to the logging system.
func (newConn handleNewConnection) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	remote, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		return
	}

	peerLogger := kitlog.With(newConn.logger, "peer", remote.ShortRef())

	// check tunnel.isRoom
	var yes bool
	err = edp.Async(ctx, &yes, muxrpc.TypeJSON, muxrpc.Method{"tunnel", "isRoom"})
	if err != nil || !yes {
		level.Warn(peerLogger).Log("event", "not a room", "err", err)
		return
	}

	var val interface{}
	err = edp.Async(ctx, &val, muxrpc.TypeJSON, muxrpc.Method{"tunnel", "announce"})
	if err != nil {
		level.Warn(peerLogger).Log("event", "failed to announce", "err", err)
		return
	}

	// open member updates stream
	src, err := edp.Source(ctx, muxrpc.TypeJSON, muxrpc.Method{"tunnel", "endpoints"})
	if err != nil {
		level.Warn(peerLogger).Log("event", "failed to open endpoints stream", "err", err)
		return
	}

	// hash of endpoints to deduplicate updates
	var lastUpdate []byte

	// stream updates
	for src.Next(ctx) {

		var (
			feeds []refs.FeedRef
			h     = sha256.New()
		)

		err := src.Reader(func(rd io.Reader) error {
			// wrap the mux stream and write everything to the hasher, too
			rd = io.TeeReader(rd, h)
			return json.NewDecoder(rd).Decode(&feeds)
		})
		if err != nil {
			level.Warn(peerLogger).Log("event", "failed to read from endpoints", "err", err)
			break
		}

		thisUpdate := h.Sum(nil)
		if !bytes.Equal(lastUpdate, thisUpdate) {
			lastUpdate = thisUpdate
			level.Info(peerLogger).Log("event", "endpoints changed", "edps", len(feeds))

			for _, f := range feeds {
				level.Info(peerLogger).Log("endpoint", f.Ref())
			}
		}
	}

	if err := src.Err(); err != nil {
		level.Error(peerLogger).Log("event", "endpoints stream closed", "err", err)
	}
}
