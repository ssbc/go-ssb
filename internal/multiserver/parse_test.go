// SPDX-License-Identifier: MIT

package multiserver

import (
	"net"
	"strings"
	"testing"

	"go.cryptoscope.co/ssb"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func mustParseFeedRef(ref string) *ssb.FeedRef {
	r, err := ssb.ParseFeedRef(ref)
	if err != nil {
		panic(err)
	}
	return r
}

func TestParseNetAddress(t *testing.T) {

	type tcase struct {
		name        string
		input       string
		hasMultiple bool
		inputIdx    int
		want        *NetAddress
		err         error
	}

	var cases = []tcase{
		{
			name:  "simple",
			input: "net:192.168.1.137:8008~shs:e84qV/tx9w1ZiOIxU3+fOpirrT8rP3YqDydRgfk076c=",
			want: &NetAddress{
				Addr: net.TCPAddr{
					IP:   net.ParseIP("192.168.1.137"),
					Port: 8008,
				},
				Ref: mustParseFeedRef("@e84qV/tx9w1ZiOIxU3+fOpirrT8rP3YqDydRgfk076c=.ed25519"),
			},
		},
		{
			name:  "net-last",
			input: "ws://192.168.1.171:8989~shs:EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=;net:192.168.1.171:8008~shs:EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=",
			want: &NetAddress{
				Addr: net.TCPAddr{
					IP:   net.ParseIP("192.168.1.171"),
					Port: 8008,
				},
				Ref: mustParseFeedRef("@EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=.ed25519"),
			},
			hasMultiple: true,
			inputIdx:    1,
		},

		// {
		// 	name:  "net-with-hostname",
		// 	input: "net:localhost:12352~shs:x9a730cuA8I83lxfkYo0eewzaojxWryhDm07hVqnnLY=",
		// 	want: &NetAddress{
		// 		Addr: net.TCPAddr{
		// 			IP:   net.ParseIP("127.0.0.1"),
		// 			Port: 12352,
		// 		},
		// 		Ref: mustParseFeedRef("@x9a730cuA8I83lxfkYo0eewzaojxWryhDm07hVqnnLY=.ed25519"),
		// 	},
		// },

		{
			name:  "no-net",
			input: "ws://192.168.1.171:8989~shs:EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=",
			err:   ErrNoNetAddr,
		},
		{
			name:  "no-net-just-unix",
			input: "unix:/home/some1/.ssb/socket~noauth",
			err:   ErrNoNetAddr,
		},
		{
			name:  "b0rked key",
			input: "net:10.10.0.1:8008~shs:invalid",
			err:   ErrNoSHSKey,
		},
		//{
		//	name:  "weird v6",
		//	input: `net:fe80::14fe:529f:e269:7796:8008~shs:p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=;ws://[::]:8989~shs:p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=`,
		//	// _could_ error this since shoudl really be [v6]:port, no?
		//	want: &NetAddress{
		//		Host: net.ParseIP("fe80::14fe:529f:e269:7796"),
		//		Port: 8008,
		//		Ref:  mustParseFeedRef("@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519"),
		//	},
		//},
		{
			name:  "valid v6",
			input: `net:[fe80::beee:7bff:fe8c:6ffc]:8008~shs:p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=;ws://[::]:8989~shs:p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=`,
			want: &NetAddress{
				Addr: net.TCPAddr{
					IP:   net.ParseIP("fe80::beee:7bff:fe8c:6ffc"),
					Port: 8008,
					// Zone: "en0", TODO add zone tests
				},
				Ref: mustParseFeedRef("@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519"),
			},
			hasMultiple: true,
			inputIdx:    0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			addr, err := ParseNetAddress([]byte(tc.input))
			if tc.err == nil {
				r.NoError(err)
				r.Equal(tc.want, addr)
				if tc.hasMultiple {
					split := strings.Split(tc.input, ";")
					tc.input = split[tc.inputIdx]
				}
				r.Equal(tc.input, addr.String())
			} else {
				r.Equal(errors.Cause(err), tc.err)
				r.Nil(addr)
			}
		})
	}
}
