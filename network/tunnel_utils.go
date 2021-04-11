// SPDX-License-Identifier: MIT

package network

import (
	"fmt"
	"io"
	"net"
	"time"

	refs "go.mindeco.de/ssb-refs"
)

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
