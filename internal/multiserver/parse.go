// first draft of an (surely) incomlete implemenation of multiserver addresses
package multiserver

import (
	"bytes"
	"encoding/base64"
	"net"

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
			if bytes.HasSuffix(netPart, []byte(":8008")) {
				host, _, _ := net.SplitHostPort(string(netPart))
				na.Host = net.ParseIP(host)
				if na.Host == nil {
					return nil, errors.Wrap(ErrNoNetAddr, "multiserver: no valid IP in net: section")
				}
				na.Port = 8008
			} else { // regexp portnumber?
				return nil, errors.Errorf("unusual net: %s", string(netPart))
			}

			keyBytes := make([]byte, 35)
			n, err := base64.StdEncoding.Decode(keyBytes, shsPart)
			if err != nil {
				return nil, errors.Wrapf(ErrNoSHSKey, "multiserver: invalid pubkey formatting: %s", err)
			}
			if n != 32 {
				return nil, errors.Wrap(ErrNoSHSKey, "multiserver: pubkey not 32bytes long")
			}
			na.Ref = &ssb.FeedRef{
				Algo: ssb.RefAlgoEd25519, // implied by ~shs: indicating v1
				ID:   keyBytes[:32],
			}
			return &na, nil
		}
	}
	return nil, ErrNoNetAddr
}
