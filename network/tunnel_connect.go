package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/muxrpc/v2/typemux"
	"go.cryptoscope.co/ssb"
)

func (n *node) TunnelPlugin() ssb.Plugin {
	rootHdlr := typemux.New(n.log)

	rootHdlr.RegisterAsync(muxrpc.Method{"tunnel", "isRoom"}, isRoomhandler{})
	rootHdlr.RegisterDuplex(muxrpc.Method{"tunnel", "connect"}, connectHandler{
		network: n,
		// builder: b,
		// self:    self,
	})

	return plugin{
		h: handleNewConnection{
			Handler: &rootHdlr,
		},
	}
}

type plugin struct{ h muxrpc.Handler }

func (plugin) Name() string              { return "tunnel" }
func (plugin) Method() muxrpc.Method     { return muxrpc.Method{"tunnel"} }
func (p plugin) Handler() muxrpc.Handler { return p.h }

type isRoomhandler struct{}

func (h isRoomhandler) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	return false, nil
}

type connectHandler struct {
	network *node
}

func (h connectHandler) HandleDuplex(ctx context.Context, req *muxrpc.Request, peerSrc *muxrpc.ByteSource, peerSnk *muxrpc.ByteSink) error {
	fmt.Println("tunnel.connect called from:")
	fmt.Println(string(req.RawArgs))

	// TODO: turn peerSrc and peerSnk into a rwc

	// wrap and authenticate
	var tc tunnelConn
	tc.Reader = muxrpc.NewSourceReader(peerSrc)
	tc.WriteCloser = muxrpc.NewSinkWriter(peerSnk)

	tc.local = h.network.opts.ListenAddr
	// TODO: should be tunnel:portal
	var err error
	tc.remote, err = net.ResolveTCPAddr("tcp4", "10.0.0.23:8008")
	if err != nil {
		return err
	}

	authWrapper := h.network.secretServer.ConnWrapper()

	conn, err := authWrapper(tc)
	if err != nil {
		return err
	}
	remote, err := ssb.GetFeedRefFromAddr(conn.RemoteAddr())
	if err != nil {
		return err
	}

	fmt.Println("tunnel.connect authentication worked with:", remote.ShortRef())
	go h.network.handleConnection(ctx, conn, false)

	return nil
}

type tunnelConn struct {
	local, remote net.Addr

	io.Reader
	io.WriteCloser
}

func (c tunnelConn) LocalAddr() net.Addr  { return c.local }
func (c tunnelConn) RemoteAddr() net.Addr { return c.remote }

func (c tunnelConn) SetDeadline(t time.Time) error {
	return nil // c.conn.SetDeadline(t)
}
func (c tunnelConn) SetReadDeadline(t time.Time) error {
	return nil // c.conn.SetReadDeadline(t)
}
func (c tunnelConn) SetWriteDeadline(t time.Time) error {
	return nil // c.conn.SetWriteDeadline(t)
}

type handleNewConnection struct {
	muxrpc.Handler
}

func (newConn handleNewConnection) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	remote, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		return
	}

	var yes bool
	err = edp.Async(ctx, &yes, muxrpc.TypeJSON, muxrpc.Method{"tunnel", "isRoom"})
	if err != nil || !yes {
		fmt.Println(remote.ShortRef(), "not a room", err, yes)
		return
	}

	src, err := edp.Source(ctx, muxrpc.TypeJSON, muxrpc.Method{"tunnel", "endpoints"})
	if err != nil {
		fmt.Println(remote.ShortRef(), "failed to open endpoints", err)
		return
	}

	for src.Next(ctx) {

		endpoints, err := src.Bytes()
		if err != nil {
			return
		}

		fmt.Println(remote.ShortRef(), ": ", string(endpoints))
	}

	if err := src.Err(); err != nil {
		fmt.Println("endpoints stream errored:", err)
	}
}
