// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package slp

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
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
			in:   [][]byte{{0, 8, 16, 32}},
			out:  []byte{4, 0, 0, 8, 16, 32},
		},
		{
			name: "pair",
			in:   [][]byte{[]byte("asd"), []byte("def")},
			out:  []byte{3, 0, 'a', 's', 'd', 3, 0, 'd', 'e', 'f'},
		},
		{
			name: "pair-binary",
			in:   [][]byte{{0, 8, 16, 32}, {4, 12, 20, 36}},
			out:  []byte{4, 0, 0, 8, 16, 32, 4, 0, 4, 12, 20, 36},
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			data, err := Encode(tc.in...)
			require.NoError(t, err)
			require.Equal(t, tc.out, data)

			require.Equal(t, tc.in, Decode(tc.out))
		})
	}
}

func TestEncodeTooLarge(t *testing.T) {
	tooLargeElement := bytes.Repeat([]byte("A"), math.MaxUint16+1)
	out, err := Encode(tooLargeElement)
	require.Error(t, err)
	require.Nil(t, out)
}
