package network

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/pkg/errors"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/multiserver"
)

type Discoverer struct {
	local *ssb.KeyPair // to ignore our own

	rx net.PacketConn

	brLock    sync.Mutex
	brodcasts map[int]chan net.Addr
}

func NewDiscoverer(local *ssb.KeyPair) (*Discoverer, error) {
	d := &Discoverer{
		local:     local,
		brodcasts: make(map[int]chan net.Addr),
	}
	return d, d.start()
}

func (d *Discoverer) start() error {

	var err error
	lis, err := reuseport.ListenPacket("udp", fmt.Sprintf("%s:%d", net.IPv4bcast, DefaultPort))
	if err != nil {
		return errors.Wrap(err, "ssb: adv start failed to listen on v4 broadcast")
	}
	switch v := lis.(type) {
	case *net.UDPConn:
		d.rx = v
	default:
		return errors.Errorf("node Advertise: invalid rx listen type: %T", lis)
	}

	go func() {

		for {
			d.rx.SetReadDeadline(time.Now().Add(time.Second * 1))
			buf := make([]byte, 128)
			n, addr, err := d.rx.ReadFrom(buf)
			if err != nil {
				if !os.IsTimeout(err) {
					log.Printf("rx adv err, breaking (%s)", err.Error())
					break
				}
				continue
			}

			buf = buf[:n] // strip of zero bytes

			log.Printf("dbg adv raw: %q", string(buf))
			na, err := multiserver.ParseNetAddress(buf)
			if err != nil {
				log.Println("rx adv err", err.Error())
				// TODO: _could_ try to get key out if just ws://[::]~shs:... and dial pkt origin
				continue
			}

			// if bytes.Equal(na.Ref.ID, d.local.Id.ID) {
			// 	continue
			// }

			// ua := addr.(*net.UDPAddr)
			// if d.local.IP.Equal(ua.IP) {
			// 	// ignore same origin
			// 	continue
			// }

			log.Printf("[localadv debug] %s (claimed:%s %d) %s", addr, na.Host.String(), na.Port, na.Ref.Ref())

			// TODO: check if adv.Host == addr ?
			wrappedAddr := netwrap.WrapAddr(&net.TCPAddr{
				IP: na.Host,
				// IP:   ua.IP,
				Port: na.Port,
			}, secretstream.Addr{PubKey: na.Ref.ID})
			d.brLock.Lock()
			for _, ch := range d.brodcasts {
				ch <- wrappedAddr
			}
			d.brLock.Unlock()
		}
	}()

	return nil
}

func (d *Discoverer) Stop() {
	d.brLock.Lock()
	for i, ch := range d.brodcasts {
		close(ch)
		delete(d.brodcasts, i)
	}
	if d.rx != nil {
		d.rx.Close()
		d.rx = nil
	}
	d.brLock.Unlock()
	return
}

func (d *Discoverer) Notify() (<-chan net.Addr, func()) {
	ch := make(chan net.Addr)
	d.brLock.Lock()
	i := len(d.brodcasts)
	d.brodcasts[i] = ch
	d.brLock.Unlock()
	return ch, func() {
		d.brLock.Lock()
		_, open := d.brodcasts[i]
		if open {
			close(ch)
			delete(d.brodcasts, i)
		}
		d.brLock.Unlock()
	}
}
