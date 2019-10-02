package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/keys"
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
	fref := feedRefFromTFK(ut.Input.FeedID)
	mref := messageRefFromTFK(ut.Input.PrevMsgID)

	keySlots, _ := deriveMessageKey(fref, mref, []keys.Recipient{
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
