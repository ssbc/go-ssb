package netwraputil

import (
	"net"

	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
)

// SpoofRemoteAddress wraps the connection with the passed reference
// as if it was a secret-handshake connection
// warning: should only be used where auth is established otherwise,
// like for testing or local client access over unixsock
func SpoofRemoteAddress(remote *ssb.FeedRef) netwrap.ConnWrapper {
	return func(c net.Conn) (net.Conn, error) {
		var spoofedAddr secretstream.Addr
		spoofedAddr.PubKey = make([]byte, 32)
		copy(spoofedAddr.PubKey, remote.ID)

		sc := SpoofedConn{
			Conn:          c,
			spoofedRemote: netwrap.WrapAddr(c.RemoteAddr(), spoofedAddr),
		}
		return sc, nil
	}
}

type SpoofedConn struct {
	net.Conn

	spoofedRemote net.Addr
}

func (sc SpoofedConn) RemoteAddr() net.Addr {
	return sc.spoofedRemote
}
