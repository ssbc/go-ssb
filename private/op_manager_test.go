// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package private

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"
)

var testMessage = json.RawMessage(`{"type":"test", "some": 1, "msg": "here"}`)

type OpManagerEncryptBox1 struct {
	Manager *Manager

	Recipients []refs.FeedRef

	Ciphertext *[]byte

	ExpErr string
}

func (op OpManagerEncryptBox1) Do(t *testing.T, _ interface{}) {

	// encrypt
	ctxt, err := op.Manager.EncryptBox1(testMessage, op.Recipients...)
	expErr(t, err, op.ExpErr, "encrypt")

	*op.Ciphertext = ctxt
}

type OpManagerEncryptBox2 struct {
	Manager *Manager

	Prev       *refs.MessageRef
	Recipients []refs.Ref

	Ciphertext *[]byte

	ExpErr string
}

func (op OpManagerEncryptBox2) Do(t *testing.T, _ interface{}) {
	prev := getZeroPrev(t, op.Prev)

	// encrypt
	ctxt, err := op.Manager.EncryptBox2(testMessage, prev, op.Recipients)
	expErr(t, err, op.ExpErr, "encrypt")

	*op.Ciphertext = ctxt
}

type OpManagerDecryptBox1 struct {
	Manager    *Manager
	Ciphertext *[]byte

	ExpDecryptErr string
}

func (op OpManagerDecryptBox1) Do(t *testing.T, _ interface{}) {

	// attempt decryption
	dec, err := op.Manager.DecryptBox1(*op.Ciphertext)
	expErr(t, err, op.ExpDecryptErr, "decrypt")

	require.EqualValues(t, testMessage, dec, "msg decrypted not equal")
}

type OpManagerDecryptBox2 struct {
	Manager    *Manager
	Ciphertext *[]byte
	Sender     refs.FeedRef
	Previous   *refs.MessageRef

	ExpDecryptErr string
}

func (op OpManagerDecryptBox2) Do(t *testing.T, _ interface{}) {
	prev := getZeroPrev(t, op.Previous)

	// attempt decryption
	dec, err := op.Manager.DecryptBox2(*op.Ciphertext, op.Sender, prev)
	expErr(t, err, op.ExpDecryptErr, "decrypt")

	require.EqualValues(t, testMessage, dec, "msg decrypted not equal")
}

// expErr uses either require.NoError or require.EqualError, depending on
// whether the expErr arguemnt is the empty string or not
func expErr(t *testing.T, err error, expErr string, comment string) {
	if expErr == "" {
		require.NoError(t, err, comment)
	} else {
		require.EqualError(t, err, expErr, comment)
	}
}

func getZeroPrev(t *testing.T, input *refs.MessageRef) refs.MessageRef {
	var prev refs.MessageRef
	if input == nil {
		zeroBytes := bytes.Repeat([]byte{0}, 32)
		zeroPrev, err := refs.NewMessageRefFromBytes(zeroBytes, refs.RefAlgoMessageSSB1)
		if err != nil {
			t.Fatal(err)
		}
		prev = zeroPrev
	} else {
		prev = *input
	}
	return prev
}
