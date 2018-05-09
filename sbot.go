package sbot

import (
	"log"
	"net"
	"sync"

	"cryptoscope.co/go/muxrpc"
	"github.com/pkg/errors"
)

// TODO: move halt stuff to new package curfew
//       and add nesting helper types
type HaltOpts struct {
	// Force shutdown instead of doing it gracefully.
	Force bool
}

type Haltable interface {
	// Halt makes the callee shut down.
	Halt(opts HaltOpts) error
}

type Repo interface {
	// Path returns the absolute path, given a path relative to the repository root.
	Path(name string) string
}

type Options struct {
	Listener    net.Listener
	MakeHandler func(net.Conn) muxrpc.Handler
}

type Node interface {
	Haltable
	Repo

	Connect(addr net.Addr) error
	Serve() error
}

type node struct {
	opts Options

	activeLock sync.Mutex
	active     map[activeConn]struct{}
}

type activeConn struct {
	// these are basically net.Addr's, but we take the .String() because that's better for map keys
	Local, Remote string
}

func toActiveConn(conn net.Conn) activeConn {
	return activeConn{
		Local:  conn.LocalAddr().String(),
		Remote: conn.RemoteAddr().String(),
	}
}

func NewNode(opts Options) Node {
	return &node{
		opts:   opts,
		active: make(map[activeConn]struct{}),
	}
}

func (n *node) Serve() error {
	for {
		c, err := n.opts.Listener.Accept()
		if err != nil {
			return errors.Wrap(err, "errors accepting connection")
		}
	}

	h := n.opts.MakeHandler(c)
	pkr := muxrpc.NewPacker(c)

	go func() {
		func() {
			n.activeLock.Lock()
			defer n.activeLock.Unlock()

			n.active[toActiveConn(conn)] = struct{}{}
		}

		defer func() {
			n.activeLock.Lock()
			defer n.activeLock.Unlock()

			delete(n.active, toActiveConn(conn))
		}()

		err := Handle(pkr, h)
		if err != nil {
			log.Printf("error handling connection with peer %q: %s", conn.RemoteAddr(), err)
		}
	}()
}
