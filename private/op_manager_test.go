package private

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
)

type OpManagerEncrypt struct {
	Manager    *Manager
	Message    *interface{}
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

	Message interface{}

	ExpDecryptErr string
	ExpBase64Err  string
	ExpJSONErr    string
	ExpMessage    interface{}
}

func (op OpManagerDecrypt) Do(t *testing.T, env interface{}) {
	ctx := context.TODO()

	// check that output field is a pointer
	rv := reflect.ValueOf(op.Message)
	require.Equal(t, rv.Kind(), reflect.Ptr, "output not a pointer")

	// attempt decryption
	err := op.Manager.Decrypt(ctx, op.Message, *op.Ciphertext, op.Sender, op.Options...)
	expErr(t, err, op.ExpDecryptErr, "decrypt")

	// check that ExpMessage == *op.Message
	// (which doesn't work directly becuause pointers and interfaces).
	require.Equal(t, op.ExpMessage, rv.Elem().Interface(), "msg decrypted not equal")
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
