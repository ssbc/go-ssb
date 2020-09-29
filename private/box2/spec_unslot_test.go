package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/keys"
	"go.mindeco.de/ssb-refs/tfk"
)

type unslotSpecTest struct {
	genericSpecTest

	Input  unslotSpecTestInput  `json:"input"`
	Output unslotSpecTestOutput `json:"output"`
}

type unslotSpecTestInput struct {
	KeySlot   b64str `json:"key_slot"`
	FeedID    b64str `json:"feed_id"`
	PrevMsgID b64str `json:"prev_msg_id"`
	Recipient struct {
		Key    b64str         `json:"key"`
		Scheme keys.KeyScheme `json:"scheme"`
	} `json:"recipient"`
}

type unslotSpecTestOutput struct {
	MessageKey []byte `json:"msg_key"`
}

func (ut unslotSpecTest) Test(t *testing.T) {
	var f tfk.Feed
	err := f.UnmarshalBinary(ut.Input.FeedID)
	require.NoError(t, err)

	var m tfk.Message
	err = m.UnmarshalBinary(ut.Input.PrevMsgID)
	require.NoError(t, err)

	keySlots, _ := deriveMessageKey(f.Feed(), m.Message(), []keys.Recipient{
		{Key: keys.Key(ut.Input.Recipient.Key), Scheme: ut.Input.Recipient.Scheme},
	})

	require.Len(t, keySlots, 1)

	// xor to get the message key

	msgKey := make([]byte, KeySize)

	for idx := range keySlots[0] {
		msgKey[idx] = keySlots[0][idx] ^ ut.Input.KeySlot[idx]
	}

	require.Equal(t, ut.Output.MessageKey, msgKey)
}
