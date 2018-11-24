package ssb

import (
	"net"
	"sync"
	"time"

	"github.com/go-kit/kit/metrics"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
)

type instrumentedConnTracker struct {
	root ConnTracker

	count     metrics.Gauge
	durration metrics.Histogram
}

func NewInstrumentedConnTracker(r ConnTracker, ct metrics.Gauge, h metrics.Histogram) ConnTracker {
	i := instrumentedConnTracker{root: r, count: ct, durration: h}
	return &i
}

func (ict instrumentedConnTracker) Count() uint {
	n := ict.root.Count()
	ict.count.With("part", "tracked_count").Set(float64(n))
	return n
}

func (ict instrumentedConnTracker) OnAccept(conn net.Conn) bool {
	ok := ict.root.OnAccept(conn)
	if ok {
		ict.count.With("part", "tracked_conns").Add(1)
	}
	return ok
}

func (ict instrumentedConnTracker) OnClose(conn net.Conn) time.Duration {
	durr := ict.root.OnClose(conn)
	if durr > 0 {
		ict.count.With("part", "tracked_conns").Add(-1)
		ict.durration.With("part", "tracked_conns").Observe(durr.Seconds())
	}
	return durr
}

type ConnTracker interface {
	OnAccept(conn net.Conn) bool
	OnClose(conn net.Conn) time.Duration
	Count() uint
}

func NewConnTracker() ConnTracker {
	return &connTracker{active: make(map[[32]byte]time.Time)}
}

// tracks open connections and refuses to established pubkeys
type connTracker struct {
	activeLock sync.Mutex
	active     map[[32]byte]time.Time
}

func (ct *connTracker) Count() uint {
	ct.activeLock.Lock()
	defer ct.activeLock.Unlock()
	return uint(len(ct.active))
}

func toActive(a net.Addr) [32]byte {
	shs, ok := netwrap.GetAddr(a, "shs-bs").(secretstream.Addr)
	if !ok {
		panic("ssb:what")
	}
	var pk [32]byte
	copy(pk[:], shs.PubKey)
	return pk
}

func (ct *connTracker) OnAccept(conn net.Conn) bool {
	ct.activeLock.Lock()
	defer ct.activeLock.Unlock()
	k := toActive(conn.RemoteAddr())
	_, ok := ct.active[k]
	if ok {
		return false
	}
	ct.active[k] = time.Now()
	return true
}

func (ct *connTracker) OnClose(conn net.Conn) time.Duration {
	ct.activeLock.Lock()
	defer ct.activeLock.Unlock()

	k := toActive(conn.RemoteAddr())
	when, ok := ct.active[k]
	if !ok {
		return 0
	}
	delete(ct.active, k)
	return time.Since(when)
}
