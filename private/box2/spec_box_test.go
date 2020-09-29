package box2

import (
	"bytes"
	"testing"

	"go.mindeco.de/ssb-refs/tfk"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/keys"
)

type boxSpecTest struct {
	genericSpecTest

	Input  boxSpecTestInput  `json:"input"`
	Output boxSpecTestOutput `json:"output"`
}

type boxSpecTestInput struct {
	PlainText []byte `json:"plain_text"`
	FeedID    b64str `json:"feed_id"` // as TFK
	PrevMsgID b64str `json:"prev_msg_id"`
	MsgKey    []byte `json:"msg_key"`
	RecpKeys  []struct {
		Key    b64str         `json:"key"`
		Scheme keys.KeyScheme `json:"scheme"`
	} `json:"recp_keys"`
}

type boxSpecTestOutput struct {
	Ciphertext []byte `json:"ciphertext"`
}

func (bt boxSpecTest) Test(t *testing.T) {

	rand := bytes.NewBuffer(bt.Input.MsgKey)
	bxr := NewBoxer(rand)

	recps := make([]keys.Recipient, len(bt.Input.RecpKeys))
	for i := range recps {
		recps[i] = keys.Recipient{Key: keys.Key(bt.Input.RecpKeys[i].Key), Scheme: bt.Input.RecpKeys[i].Scheme}
	}

	var f tfk.Feed
	err := f.UnmarshalBinary(bt.Input.FeedID)
	require.NoError(t, err)

	var m tfk.Message
	err = m.UnmarshalBinary(bt.Input.PrevMsgID)
	require.NoError(t, err)

	out, err := bxr.Encrypt(
		nil,
		bt.Input.PlainText,
		f.Feed(),
		m.Message(),
		recps,
	)

	if len(bt.Input.PlainText) == 0 || bt.ErrorCode != nil {
		require.Error(t, err, "error case but passed: %s", bt.ErrorCode)
		require.Nil(t, out, "output in error case %s", bt.ErrorCode)
	} else {
		require.NoError(t, err)
		require.Equal(t, bt.Output.Ciphertext, out)
	}
}
