// SPDX-License-Identifier: MIT

package private

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
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

	kp, err := ssb.NewKeyPair(nil)
	r.NoError(err)

	for i, msg := range tmsgs {
		sbox, err := Box(msg, kp.Id)
		r.NoError(err, "failed to create ciphertext %d", i)
		sbox = sbox[5:]
		out, err := Unbox(kp, sbox)
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

	who, err := ssb.NewKeyPair(nil)
	r.NoError(err)

	kp, err := ssb.NewKeyPair(nil)
	r.NoError(err)

	for i, msg := range tmsgs {
		sbox, err := Box(msg, who.Id)
		r.NoError(err, "failed to create ciphertext %d", i)

		out, err := Unbox(kp, sbox)
		r.Error(err)
		r.Nil(out)
	}
}
