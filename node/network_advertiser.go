package node

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
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

func newAdvertisement(local *net.UDPAddr, keyPair *ssb.KeyPair) (string, error) {
	if local == nil {
		return "", errors.Errorf("ssb: passed nil local address")
	}

	withoutZone := *local
	withoutZone.Zone = ""

	// crunchy way of making a https://github.com/ssbc/multiserver/
	msg := fmt.Sprintf("net:%s~shs:%s", &withoutZone, newPublicKeyString(keyPair))
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
		if !isIPv4(nv.IP) {
			udpAddr.Zone = nv.Zone
		}
	case *net.UDPAddr:
		udpAddr = nv
	default:
		return nil, errors.Errorf("node Advertise: invalid local address type: %T", local)
	}
	log.Printf("adverstiser using local address %s", udpAddr)

	remote, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", net.IPv4bcast, ssb.DefaultPort))
	if err != nil {
		return nil, errors.Wrap(err, "ssb/NewAdvertiser: failed to resolve v4 broadcast addr")
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
	localAddresses, err := findSiteLocalNetworkAddresses(b.local)
	if err != nil {
		return errors.Wrap(err, "ssb: failed to make new advertisment")
	}

	for _, localAddress := range localAddresses {
		// log.Print("DBG23: using", localAddress)
		var localUDP = new(net.UDPAddr)
		// carry port from address or use default one
		switch v := localAddress.(type) {
		case *net.IPAddr:
			localUDP.IP = v.IP
			if !isIPv4(v.IP) {
				localUDP.Zone = v.Zone
			}
			localUDP.Port = ssb.DefaultPort
		case *net.IPNet:
			localUDP.IP = v.IP
			localUDP.Port = ssb.DefaultPort
		case *net.TCPAddr:
			localUDP.IP = v.IP
			localUDP.Port = v.Port
		case *net.UDPAddr:
			localUDP.IP = v.IP
			localUDP.Port = v.Port
		default:
			return errors.Errorf("cannot get Port for network type %s", localAddress.Network())
		}

		broadcastAddress, err := localBroadcastAddress(localAddress)
		if err != nil {
			return errors.Wrap(err, "ssb: failed to find site local address broadcast address")
		}
		dstStr := net.JoinHostPort(broadcastAddress, strconv.Itoa(ssb.DefaultPort))
		remoteUDP, err := net.ResolveUDPAddr("udp", dstStr)
		if err != nil {
			return errors.Wrapf(err, "ssb: failed to resolve broadcast dest addr for advertiser: %s", dstStr)
		}

		msg, err := newAdvertisement(
			localUDP,
			b.keyPair,
		)
		if err != nil {
			return err
		}
		broadcastConn, err := reuseport.Dial("udp", localUDP.String(), remoteUDP.String())
		if err != nil {
			log.Println("adv dial failed", err.Error())
			continue
		}
		_, err = fmt.Fprint(broadcastConn, msg)
		_ = broadcastConn.Close()
		if err != nil {
			log.Println(err.Error())
		}
	}
	return nil
}

func (b *Advertiser) Start() error {
	b.ticker = time.NewTicker(b.waitTime)
	// TODO: notice interface changes
	// net.IPv6linklocalallnodes
	var err error
	lis, err := reuseport.ListenPacket("udp", fmt.Sprintf("%s:%d", net.IPv4bcast, ssb.DefaultPort))
	if err != nil {
		return errors.Wrap(err, "ssb: adv start failed to listen on v4 broadcast")
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

			if bytes.Equal(na.Ref.ID, b.keyPair.Id.ID) {
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
				IP: na.Host,
				// IP:   ua.IP,
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
	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
	b.brLock.Unlock()
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
