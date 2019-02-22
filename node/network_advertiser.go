package node

import (
	"encoding/base64"
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

type Advertiser struct {
	keyPair *ssb.KeyPair

	conn   net.PacketConn
	local  *net.UDPAddr // Local listening address, may not be needed (auto-detect?).
	remote *net.UDPAddr // Address being broadcasted to, this should be deduced form 'local'.

	waitTime time.Duration
	ticker   *time.Ticker

	brLock    sync.Mutex
	brodcasts map[int]chan net.Addr
}

func newPublicKeyString(keyPair *ssb.KeyPair) string {
	publicKey := keyPair.Pair.Public[:]
	return base64.StdEncoding.EncodeToString(publicKey)
}

// TODO: Fix sourcing of address
func newAdvertisement(local *net.UDPAddr, keyPair *ssb.KeyPair) (string, error) {
	if local == nil {
		return "", errors.Errorf("ssb: passed nil local address")
	}
	// crunchy way of making a https://github.com/ssbc/multiserver/
	msg := fmt.Sprintf("net:%s~shs:%s", local, newPublicKeyString(keyPair))
	_, err := multiserver.ParseNetAddress([]byte(msg))
	return msg, err
}

func NewAdvertiser(local net.Addr, keyPair *ssb.KeyPair) (*Advertiser, error) {

	var udpAddr *net.UDPAddr
	switch nv := local.(type) {
	case *net.TCPAddr:
		udpAddr = new(net.UDPAddr)
		udpAddr.IP = nv.IP
		udpAddr.Port = nv.Port
		udpAddr.Zone = nv.Zone
	case *net.UDPAddr:
		udpAddr = nv
	default:
		return nil, errors.Errorf("node Advertise: invalid local address type: %T", local)
	}
	log.Printf("adverstiser using local address %s", udpAddr)

	remote, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", net.IPv4bcast, ssb.DefaultPort))
	if err != nil {
		return nil, errors.Wrap(err, "ssb: failed to resolve addr for advertiser")
	}

	return &Advertiser{
		local:     udpAddr,
		remote:    remote,
		waitTime:  time.Second * 1,
		keyPair:   keyPair,
		brodcasts: make(map[int]chan net.Addr),
	}, nil
}

func (b *Advertiser) advertise() error {
	msg, err := newAdvertisement(
		b.local,
		b.keyPair,
	)
	if err != nil {
		return errors.Wrap(err, "ssb: failed to make new advertisment")
	}
	_, err = b.conn.WriteTo([]byte(msg), b.remote)
	return errors.Wrap(err, "ssb: could not send advertisement")
}

func (b *Advertiser) Start() error {
	b.ticker = time.NewTicker(b.waitTime)
	var err error

	lis, err := reuseport.ListenPacket("udp4", fmt.Sprintf("%s:%d", net.IPv4bcast, ssb.DefaultPort))
	if err != nil {
		return errors.Wrap(err, "ssb: could not on local address")
	}
	switch v := lis.(type) {
	case *net.UDPConn:
		b.conn = v
	default:
		return errors.Errorf("node Advertise: invalid rx listen type: %T", lis)
	}
	go func() {
		for range b.ticker.C {
			err := b.advertise()
			if err != nil {
				if !os.IsTimeout(err) {
					log.Printf("tx adv err, breaking (%s)", err.Error())
					break
				}
			}
		}
	}()

	go func() {

		for {
			b.conn.SetReadDeadline(time.Now().Add(time.Second * 1))
			buf := make([]byte, 128)
			n, addr, err := b.conn.ReadFrom(buf)
			if err != nil {
				if !os.IsTimeout(err) {
					log.Printf("rx adv err, breaking (%s)", err.Error())
					break
				}
				continue
			}

			buf = buf[:n] // strip of zero bytes
			// log.Printf("raw: %q", string(buf))
			na, err := multiserver.ParseNetAddress(buf)
			if err != nil {
				// log.Println("rx adv err", err.Error())
				// TODO: _could_ try to get key out if just ws://[::]~shs:... and dial pkt origin
				continue
			}

			ua := addr.(*net.UDPAddr)
			if b.local.IP.Equal(ua.IP) {
				// ignore same origin
				continue
			}

			log.Printf("[localadv debug] %s (claimed:%s %d) %s", addr, na.Host.String(), na.Port, na.Ref.Ref())

			// TODO: check if adv.Host == addr ?
			wrappedAddr := netwrap.WrapAddr(&net.TCPAddr{
				// IP:   na.Host,
				IP:   ua.IP,
				Port: na.Port,
			}, secretstream.Addr{PubKey: na.Ref.ID})
			b.brLock.Lock()
			for _, ch := range b.brodcasts {
				ch <- wrappedAddr
			}
			b.brLock.Unlock()
		}
	}()
	return nil
}

func (b *Advertiser) Stop() {
	b.ticker.Stop()
	b.brLock.Lock()
	for i, ch := range b.brodcasts {
		close(ch)
		delete(b.brodcasts, i)
	}
	b.brLock.Unlock()
	b.conn.Close()
}

func (b *Advertiser) Notify() (<-chan net.Addr, func()) {
	ch := make(chan net.Addr)
	b.brLock.Lock()
	i := len(b.brodcasts)
	b.brodcasts[i] = ch
	b.brLock.Unlock()
	return ch, func() {
		b.brLock.Lock()
		_, open := b.brodcasts[i]
		if open {
			close(ch)
			delete(b.brodcasts, i)
		}
		b.brLock.Unlock()
	}
}
