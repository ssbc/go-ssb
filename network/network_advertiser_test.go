// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package network

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func makeTestPubKey(t *testing.T) ssb.KeyPair {
	kp, err := ssb.NewKeyPair(strings.NewReader(strings.Repeat("bep", 32)), refs.RefAlgoFeedSSB1)
	if err != nil {
		t.Fatal(err)
	}
	return kp
}

func makeRandPubkey(t *testing.T) ssb.KeyPair {
	kp, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	if err != nil {
		t.Fatal(err)
	}
	return kp
}

func TestNewAdvertisement(t *testing.T) {
	type tcase struct {
		local       *net.UDPAddr
		keyPair     ssb.KeyPair
		Expected    string
		ExpectError bool
	}

	tests := []tcase{
		{
			local: &net.UDPAddr{
				IP:   net.IPv4(1, 2, 3, 4),
				Port: 8008,
			},
			keyPair:     makeTestPubKey(t),
			ExpectError: false,
			Expected:    "net:1.2.3.4:8008~shs:TK3a+PLY/lPBGFA+jAdq9ZjHug/Je36ARMuwQwibi5A=",
		},
	}

	for _, test := range tests {
		res, err := newAdvertisement(test.local, test.keyPair)
		if test.ExpectError {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, test.Expected, res)
	}
}

/*  TODO: test two on different networks

senderAddr, err := net.ResolveUDPAddr("udp", "192.168.42.21:8008")
r.NoError(err)

adv, err := NewAdvertiser(senderAddr, pk)
r.NoError(err)

// senderAddr, err = net.ResolveUDPAddr("udp", "127.0.0.2:8008")
// r.NoError(err)
// adv2, err := NewAdvertiser(senderAddr, pk)
// r.NoError(err)

r.NoError(adv.Start(), "couldn't start 1")
// r.NoError(adv2.Start(), "couldn't start 2")
*/
