/*
This file is part of secretstream.

secretstream is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

secretstream is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with secretstream.  If not, see <http://www.gnu.org/licenses/>.
*/

package secretstream

import (
	"encoding/base64"
	"io"
	"net"
	"time"

	"go.cryptoscope.co/netwrap"
)

const NetworkString = "shs-bs"

// Addr wrapps a net.Addr and adds the public key
type Addr struct {
	PubKey []byte
}

// Network returns NetworkString, the network id of this protocol.
// Can be used with go.cryptoscope.co/netwrap to wrap the underlying connection.
func (a Addr) Network() string {
	return NetworkString
}

func (a Addr) String() string {
	// TODO keks: is this the address format we want to use?
	return "@" + base64.StdEncoding.EncodeToString(a.PubKey) + ".ed25519"
}

// Conn is a boxstream wrapped net.Conn
type Conn struct {
	io.Reader
	io.WriteCloser
	conn net.Conn

	// public keys
	local, remote []byte
}

// Close closes the underlying net.Conn
func (conn *Conn) Close() error {
	return conn.WriteCloser.Close()
}

// LocalAddr returns the local net.Addr with the local public key
func (conn *Conn) LocalAddr() net.Addr {
	return netwrap.WrapAddr(conn.conn.LocalAddr(), Addr{conn.local})
}

// RemoteAddr returns the remote net.Addr with the remote public key
func (conn *Conn) RemoteAddr() net.Addr {
	return netwrap.WrapAddr(conn.conn.RemoteAddr(), Addr{conn.remote})
}

// SetDeadline passes the call to the underlying net.Conn
func (conn *Conn) SetDeadline(t time.Time) error {
	return conn.conn.SetDeadline(t)
}

// SetReadDeadline passes the call to the underlying net.Conn
func (conn *Conn) SetReadDeadline(t time.Time) error {
	return conn.conn.SetReadDeadline(t)
}

// SetWriteDeadline passes the call to the underlying net.Conn
func (conn *Conn) SetWriteDeadline(t time.Time) error {
	return conn.conn.SetWriteDeadline(t)
}
