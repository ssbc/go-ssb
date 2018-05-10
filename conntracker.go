package sbot

import (
	"net"
	"sync"
)

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

type ConnTracker interface {
	OnAccept(conn net.Conn)
	OnClose(conn net.Conn)
}

func NewConnTracker() ConnTracker {
	return &connTracker{active: make(map[activeConn]struct{})}
}

type connTracker struct {
	activeLock sync.Mutex
	active     map[activeConn]struct{}
}

func (ct *connTracker) OnAccept(conn net.Conn) {
	ct.activeLock.Lock()
	defer ct.activeLock.Unlock()

	ct.active[toActiveConn(conn)] = struct{}{}
}

func (ct *connTracker) OnClose(conn net.Conn) {
	ct.activeLock.Lock()
	defer ct.activeLock.Unlock()

	delete(ct.active, toActiveConn(conn))
}
