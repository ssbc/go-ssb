package ssb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignatureVerify(t *testing.T) {
	a, r := assert.New(t), require.New(t)
	n := min(20, len(testMessages))
	for i := 1; i < n; i++ {
		enc, err := EncodePreserveOrder(testMessages[i].Input)
		r.NoError(err, "encode failed")

		msgWOsig, sig, err := ExtractSignature(enc)
		r.NoError(err, "extractSig failed")
		a.Equal(SigAlgoEd25519, sig.Algo())
		a.Equal(testMessages[i].NoSig, msgWOsig)

		err = sig.Verify(msgWOsig, testMessages[i].Author)
		r.NoError(err, "verify failed")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
