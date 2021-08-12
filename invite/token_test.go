// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package invite

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"

	refs "go.mindeco.de/ssb-refs"
)

func TestParseParseLegacyToken(t *testing.T) {
	a := assert.New(t)

	testRef, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte("b00p"), 8), refs.RefAlgoFeedSSB1)
	if err != nil {
		t.Fatal(err)
	}
	var tcases = []struct {
		input string
		err   error
		want  *Token
	}{
		{"127.0.0.1:123", ErrInvalidToken, nil},
		{"127.0.0.1:39979:@w50PNhe69skpWDWwitpik/hq3hrFl6PxZY/tUK+XDpM=.ed25519", ErrInvalidToken, nil},
		{"@xxx.foo", ErrInvalidToken, nil},

		{"255.1.1.255:666:@YjAwcGIwMHBiMDBwYjAwcGIwMHBiMDBwYjAwcGIwMHA=.ed25519~AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", nil, &Token{
			Address: netwrap.WrapAddr(&net.TCPAddr{
				IP:   net.ParseIP("255.1.1.255"),
				Port: 666,
			}, secretstream.Addr{testRef.PubKey()}),
			Peer: testRef,
			Seed: [32]byte{},
		}},

		// v6
		{"[fc97:c693:8b07:f84e:cbbf:d89a:16d5:3630]:1234:@YjAwcGIwMHBiMDBwYjAwcGIwMHBiMDBwYjAwcGIwMHA=.ed25519~AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", nil, &Token{
			Address: netwrap.WrapAddr(&net.TCPAddr{
				IP:   net.ParseIP("fc97:c693:8b07:f84e:cbbf:d89a:16d5:3630"),
				Port: 1234,
			}, secretstream.Addr{testRef.PubKey()}),
			Peer: testRef,
			Seed: [32]byte{},
		}},

		// hostnames
		{"localhost:666:@YjAwcGIwMHBiMDBwYjAwcGIwMHBiMDBwYjAwcGIwMHA=.ed25519~AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", nil, &Token{
			Address: netwrap.WrapAddr(&net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 666,
			}, secretstream.Addr{testRef.PubKey()}),
			Peer: testRef,
			Seed: [32]byte{},
		}},
	}
	for i, tc := range tcases {
		tok, err := ParseLegacyToken(tc.input)
		if tc.err == nil {
			if !a.NoError(err, "got error on test %d (%v)", i, tc.input) {
				continue
			}

			a.Equal(tok.String(), tc.want.String(), "test %d input<>output failed", i)
		} else {
			a.EqualError(err, tc.err.Error(), "%d wrong error", i)
		}
	}
}
