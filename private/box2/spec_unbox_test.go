package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/keys"
)

type unboxSpecTest struct {
	genericSpecTest

	Input  unboxSpecTestInput  `json:"input"`
	Output unboxSpecTestOutput `json:"output"`
}

type unboxSpecTestInput struct {
	Ciphertext []byte `json:"ciphertext"`
	FeedID     b64str `json:"feed_id"`
	PrevMsgID  b64str `json:"prev_msg_id"`
	Recipient  struct {
		Key    b64str         `json:"key"`
		Scheme keys.KeyScheme `json:"scheme"`
	} `json:"recipient"`
}

type unboxSpecTestOutput struct {
	Plaintext []byte `json:"plain_text"`
}

func (ut unboxSpecTest) Test(t *testing.T) {
	//spew.Dump(bt)
	bxr := NewBoxer(nil)
	fref := feedRefFromTFK(ut.Input.FeedID)
	mref := messageRefFromTFK(ut.Input.PrevMsgID)

	out, err := bxr.Decrypt(
		nil,
		ut.Input.Ciphertext,
		fref,
		mref,
		[]keys.Recipient{
			{Key: keys.Key(ut.Input.Recipient.Key), Scheme: ut.Input.Recipient.Scheme},
		},
	)

	require.NoError(t, err, "failed to decrypt")

	require.Equal(t, ut.Output.Plaintext, out)
}
