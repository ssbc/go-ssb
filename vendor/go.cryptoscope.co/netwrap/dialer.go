package netwrap

import (
	"net"

	"github.com/pkg/errors"
)

// ConnWrapper wraps a network connection, e.g. to encrypt the transmitted content.
type ConnWrapper func(net.Conn) (net.Conn, error)

// Dial first opens a network connection to the supplied addr, and then applies
// all the passed connection wrappers.
func Dial(addr net.Addr, wrappers ...ConnWrapper) (net.Conn, error) {
	conn, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		return nil, errors.Wrap(err, "error dialing")
	}

	for _, cw := range wrappers {
		conn, err = cw(conn)
		if err != nil {
			return nil, errors.Wrap(err, "error wrapping connection")
		}
	}

	return conn, nil
}
