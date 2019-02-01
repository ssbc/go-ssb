package ssb

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/netwrap"
)

const BroadcastAddress = "255.255.255.255"

// TODO: Add tests
// TODO: deploy daemon somewhere...

type NetworkAdvertiser struct {
	keyPair *KeyPair

	conn   net.Conn
	local  net.Addr     // Local listening address, may not be needed (auto-detect?).
	remote *net.UDPAddr // Address being broadcasted to, this should be deduced form 'local'.

	waitTime time.Duration
	ticker   *time.Ticker
}

func newPublicKeyString(keyPair *KeyPair) string {
	publicKey := keyPair.Pair.Public[:]
	return base64.StdEncoding.EncodeToString(publicKey)
}

// TODO: Fix sourcing of address
func newAdvertisement(local net.Addr, keyPair *KeyPair) ([]byte, error) {
	// QUESTION: What is the '-shs'?
	msg := fmt.Sprintf("net:%s~shs:%s", local.String(), newPublicKeyString(keyPair))
	return []byte(msg), nil
}

func NewNetworkAdvertiser(
	local net.Addr,
	keyPair *KeyPair,
) (*NetworkAdvertiser, error) {
	local = netwrap.GetAddr(local, "udp")
	log.Println("adverstiser using local address " + local.String())
	remote, err := net.ResolveUDPAddr("udp", BroadcastAddress+":"+DefaultPort)
	if err != nil {
		return nil, err
	}
	return &NetworkAdvertiser{
		local:    local,
		remote:   remote,
		waitTime: time.Second,
		keyPair:  keyPair,
	}, nil
}

func (b *NetworkAdvertiser) advertise() error {
	msg, err := newAdvertisement(
		b.local,
		b.keyPair,
	)
	if err != nil {
		return err
	}
	_, err = b.conn.Write([]byte(msg))
	return errors.Wrap(err, "could not send advertisement")
}

func (b *NetworkAdvertiser) Start() (err error) {
	b.ticker = time.NewTicker(b.waitTime)
	b.conn, err = net.DialUDP("udp", nil, b.remote)
	if err != nil {
		return errors.Wrap(err, "could not broadcast to address")
	}

	go func() {
		for _ = range b.ticker.C {
			err := b.advertise()
			if err != nil {
				log.Println(err.Error())
			}
		}
	}()
	return nil
}

func (b *NetworkAdvertiser) Stop() {
	b.ticker.Stop()
}
