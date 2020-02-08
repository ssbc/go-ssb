package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
)

func TestEncodeList(t *testing.T) {
	type testcase struct {
		name string
		in   [][]byte
		out  []byte
	}

	tcs := []testcase{
		{
			name: "empty",
		},
		{
			name: "single",
			in:   [][]byte{[]byte("asd")},
			out:  []byte{3, 0, 'a', 's', 'd'},
		},
		{
			name: "single-binary",
			in:   [][]byte{[]byte{0, 8, 16, 32}},
			out:  []byte{4, 0, 0, 8, 16, 32},
		},
		{
			name: "pair",
			in:   [][]byte{[]byte("asd"), []byte("def")},
			out:  []byte{3, 0, 'a', 's', 'd', 3, 0, 'd', 'e', 'f'},
		},
		{
			name: "pair-binary",
			in:   [][]byte{[]byte{0, 8, 16, 32}, []byte{4, 12, 20, 36}},
			out:  []byte{4, 0, 0, 8, 16, 32, 4, 0, 4, 12, 20, 36},
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, encodeList(nil, tc.in))
		})
	}
}

func TestEncodeFeedRef(t *testing.T) {
	type testcase struct {
		name string
		in   *ssb.FeedRef
		out  []byte
	}

	tcs := []testcase{
		{
			name: "ed25519",
			in: &ssb.FeedRef{
				Algo: "ed25519",
				ID:   seq(0, 32),
			},
			out: append([]byte{typeFeed, feedFormatEd25519}, seq(0, 32)...),
		},
		{
			name: "gabby",
			in: &ssb.FeedRef{
				Algo: "gabby",
				ID:   seq(0, 32),
			},
			out: append([]byte{typeFeed, feedFormatGabbyGrove}, seq(0, 32)...),
		},
		{
			name: "unknown",
			in: &ssb.FeedRef{
				Algo: "unknown",
				ID:   seq(0, 32),
			},
			out: nil,
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, encodeFeedRef(nil, tc.in))
		})
	}
}

func TestEncodeMessageRef(t *testing.T) {
	type testcase struct {
		name string
		in   *ssb.MessageRef
		out  []byte
	}

	tcs := []testcase{
		{
			name: "sha256",
			in: &ssb.MessageRef{
				Algo: "sha256",
				Hash: seq(0, 32),
			},
			out: append([]byte{typeMessage, messageFormatSHA256}, seq(0, 32)...),
		},
		{
			name: "gabby",
			in: &ssb.MessageRef{
				Algo: "gabby",
				Hash: seq(0, 32),
			},
			out: append([]byte{typeMessage, messageFormatGabbyGrove}, seq(0, 32)...),
		},
		{
			name: "unknown",
			in: &ssb.MessageRef{
				Algo: "unknown",
				Hash: seq(0, 32),
			},
			out: nil,
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, encodeMessageRef(nil, tc.in))
		})
	}
}
