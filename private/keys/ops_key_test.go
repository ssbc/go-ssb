// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package keys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type opDBKeyEncode struct {
	Key    *idxKey
	BufLen int

	ExpData []byte
	ExpErr  string
}

func (op opDBKeyEncode) Do(t *testing.T, env interface{}) {
	var (
		data []byte
		err  error
	)

	if op.BufLen == 0 {
		data, err = op.Key.MarshalBinary()
	} else {
		data = make([]byte, op.BufLen)
		var n int64
		n, err = op.Key.Read(data)
		if err == nil {
			data = data[:int(n)]
		}
	}

	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on idxk.Encode")
		require.Equal(t, op.ExpData, data, "wrong marshaled data")
	} else {
		require.EqualError(t, err, op.ExpErr, "wrong error")
	}
}

type opDBKeyDecode struct {
	Bytes []byte

	ExpKey *idxKey
	ExpErr string
}

func (op opDBKeyDecode) Do(t *testing.T, env interface{}) {
	idxk := &idxKey{}
	err := idxk.UnmarshalBinary(op.Bytes)

	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on idxk.Unmarshal")
		require.Equal(t, op.ExpKey, idxk, "wrong key")
	} else {
		require.EqualError(t, err, op.ExpErr, "wrong error")
	}
}

type opDBKeyLen struct {
	Key    *idxKey
	ExpLen int
}

func (op opDBKeyLen) Do(t *testing.T, env interface{}) {
	l := op.Key.Len()
	require.Equal(t, op.ExpLen, l, "wrong idxKey length")
}
