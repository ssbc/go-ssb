package private

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
)

type OpManagerEncrypt struct {
	Manager    *Manager
	Message    *[]byte // TODO make this interface{}
	Recipients []ssb.Ref
	Options    []EncryptOption

	Ciphertext *[]byte

	ExpErr string
}

func (op OpManagerEncrypt) Do(t *testing.T, env interface{}) {
	ctx := context.TODO()

	// add recipients option
	encOpts := make([]EncryptOption, len(op.Options)+1)
	encOpts[0] = WithRecipients(op.Recipients...)
	copy(encOpts[1:], op.Options)

	// encrypt
	ctxt, err := op.Manager.Encrypt(ctx, *op.Message, encOpts...)
	expErr(t, err, op.ExpErr, "encrypt")

	*op.Ciphertext = ctxt
}

type OpManagerDecrypt struct {
	Manager    *Manager
	Ciphertext *[]byte
	Sender     *ssb.FeedRef
	Options    []EncryptOption

	Message *[]byte // TODO make this interface{}

	ExpDecryptErr string
	ExpBase64Err  string
	ExpMessage    []byte // TODO make this interface{}
}

func expErr(t *testing.T, err error, expErr string, comment string) {
	if expErr == "" {
		require.NoError(t, err, comment)
	} else {
		require.EqualError(t, err, expErr, comment)
	}
}

func (op OpManagerDecrypt) Do(t *testing.T, env interface{}) {
	ctx := context.TODO()

	t.Log(*op.Ciphertext)

	// TODO: figure out how to pass in the recipients.
	//       maybe don't pass them in as options??
	out, err := op.Manager.Decrypt(ctx, *op.Ciphertext, op.Sender)
	expErr(t, err, op.ExpDecryptErr, "decrypt")

	out, err = base64.StdEncoding.DecodeString(out.(string))
	expErr(t, err, op.ExpBase64Err, "base64 decode")

	require.True(t, bytes.Equal(out.([]byte), op.ExpMessage), "msg decrypted not equal")

	*op.Message = out.([]byte)
}
