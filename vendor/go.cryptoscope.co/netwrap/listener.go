package netwrap

import (
	"net"

	"github.com/pkg/errors"
)

// ListenerWrapper wraps a network listener.
type ListenerWrapper func(net.Listener) (net.Listener, error)

// NewListenerWrapper creates a new ListenerWrapper by wrapping all
// accepted connections with the supplied connection wrapper and wrapping
// the listener's address with the supplied address.
func NewListenerWrapper(addr net.Addr, connWrapper ConnWrapper) ListenerWrapper {
	return func(l net.Listener) (net.Listener, error) {
		return &listener{
			Listener: l,

			addr:        WrapAddr(l.Addr(), addr),
			connWrapper: connWrapper,
		}, nil
	}
}

// Listen first listens on the supplied address and then wraps that listener
// with all the supplied wrappers.
func Listen(addr net.Addr, wrappers ...ListenerWrapper) (net.Listener, error) {
	l, err := net.Listen(addr.Network(), addr.String())
	if err != nil {
		return nil, errors.Wrap(err, "error listening")
	}

	for _, wrap := range wrappers {
		l, err = wrap(l)
		if err != nil {
			return nil, errors.Wrap(err, "error wrapping listener")
		}
	}

	return l, nil
}

type listener struct {
	net.Listener

	addr        net.Addr
	connWrapper ConnWrapper
}

func (l *listener) Addr() net.Addr {
	return l.addr
}

func (l *listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, errors.Wrap(err, "error accepting underlying connection")
	}

	conn, err = l.connWrapper(conn)
	return conn, errors.Wrap(err, "error in listerner wrapping function")
}
