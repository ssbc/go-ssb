// first draft of an (surely) incomlete implemenation of multiserver addresses
package multiserver

import (
	"bytes"
	"encoding/base64"
	"net"
	"strconv"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
)

var (
	ErrNoNetAddr = errors.New("multiserver: no net~shs combination")
	ErrNoSHSKey  = errors.New("multiserver: no or invalid shs1 key")
)

type NetAddress struct {
	Host net.IP
	Port int
	Ref  *ssb.FeedRef
}

func ParseNetAddress(input []byte) (*NetAddress, error) {
	var na NetAddress
	for _, p := range bytes.Split(input, []byte{';'}) {
		netPrefix := []byte("net:")
		if bytes.HasPrefix(p, netPrefix) {

			// where does the pubkey reside in this
			keyStart := bytes.Index(p, []byte("~shs:"))
			if keyStart == -1 {
				return nil, ErrNoSHSKey
			}

			netPart := p[len(netPrefix):keyStart]
			shsPart := p[keyStart+5:]

			// port and address handling
			host, portStr, err := net.SplitHostPort(string(netPart))
			if err != nil {
				return nil, errors.Wrap(ErrNoNetAddr, "multiserver: no valid Host + Port combination")
			}
			na.Host = net.ParseIP(host)
			if na.Host == nil {
				ipAddr, err := net.ResolveIPAddr("ip", host)
				if err != nil {
					return nil, errors.Wrap(ErrNoNetAddr, "multiserver: failed to fallback to resolving addr")
				}
				na.Host = ipAddr.IP
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, errors.Wrap(ErrNoNetAddr, "multiserver: badly formatted port")
			}
			na.Port = port

			var keyBuf = make([]byte, 35)
			n, err := base64.StdEncoding.Decode(keyBuf, shsPart)
			if err != nil {
				return nil, errors.Wrapf(ErrNoSHSKey, "multiserver: invalid pubkey formatting: %s", err)
			}
			if n != 32 {
				return nil, errors.Wrap(ErrNoSHSKey, "multiserver: pubkey not 32bytes long")
			}

			// implied by ~shs: indicating v1
			na.Ref, err = ssb.NewFeedRefEd25519(keyBuf[:32])
			if err != nil {
				return nil, errors.Wrapf(ErrNoSHSKey, "multiserver: feedRef err %s", err)
			}
			return &na, nil
		}
	}
	return nil, ErrNoNetAddr
}
