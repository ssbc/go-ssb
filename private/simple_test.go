// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package private

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/private/box"
)

func TestSimple(t *testing.T) {
	r := require.New(t)
	var tmsgs = [][]byte{
		[]byte(`[1,2,3,4,5]`),
		[]byte(`{"some": 1, "msg": "here"}`),
		[]byte(`{"hello": true}`),
		[]byte(`"plainStringLikeABlob"`),
		[]byte(`{"hello": false}`),
		[]byte(`{"hello": true}`),
	}

	kp, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	boxer := box.NewBoxer(nil)

	for i, msg := range tmsgs {
		sbox, err := boxer.Encrypt(msg, kp.ID())
		r.NoError(err, "failed to create ciphertext %d", i)

		out, err := boxer.Decrypt(kp, sbox)
		r.NoError(err, "should decrypt my message %d", i)
		r.True(bytes.Equal(out, msg), "msg decrypted not equal %d", i)
	}
}

func TestNotForMe(t *testing.T) {
	r := require.New(t)
	var tmsgs = [][]byte{
		[]byte(`[1,2,3,4,5]`),
		[]byte(`{"some": 1, "msg": "here"}`),
		[]byte(`{"hello": true}`),
		[]byte(`"plainStringLikeABlob"`),
		[]byte(`{"hello": false}`),
		[]byte(`{"hello": true}`),
	}

	who, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	kp, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	boxer := box.NewBoxer(nil)

	for i, msg := range tmsgs {
		sbox, err := boxer.Encrypt(msg, who.ID())
		r.NoError(err, "failed to create ciphertext %d", i)

		out, err := boxer.Decrypt(kp, sbox)
		r.Error(err)
		r.Nil(out)
	}
}
