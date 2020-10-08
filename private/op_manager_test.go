package private

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"
)

var testMessage = json.RawMessage(`{"type":"test", "some": 1, "msg": "here"}`)

type OpManagerEncrypt struct {
	Manager *Manager

	Recipients []refs.Ref
	Options    []EncryptOption

	Ciphertext *[]byte

	ExpErr string
}

func (op OpManagerEncrypt) Do(t *testing.T, _ interface{}) {

	// add recipients option
	encOpts := make([]EncryptOption, len(op.Options)+1)
	encOpts[0] = WithRecipients(op.Recipients...)
	copy(encOpts[1:], op.Options)

	// encrypt
	ctxt, err := op.Manager.Encrypt(testMessage, encOpts...)
	expErr(t, err, op.ExpErr, "encrypt")

	*op.Ciphertext = ctxt
}

type OpManagerDecrypt struct {
	Manager    *Manager
	Ciphertext *[]byte
	Sender     *refs.FeedRef
	Options    []EncryptOption

	ExpDecryptErr string
}

func (op OpManagerDecrypt) Do(t *testing.T, _ interface{}) {

	// attempt decryption
	dec, err := op.Manager.Decrypt(*op.Ciphertext, op.Sender, op.Options...)
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
