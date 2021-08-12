// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package box2

import (
	"bytes"
	"testing"

	"go.mindeco.de/ssb-refs/tfk"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/private/keys"
)

type boxSpecTest struct {
	genericSpecTest

	Input  boxSpecTestInput  `json:"input"`
	Output boxSpecTestOutput `json:"output"`
}

type boxSpecTestInput struct {
	PlainText []byte            `json:"plain_text"`
	FeedID    keys.Base64String `json:"feed_id"` // as TFK
	PrevMsgID keys.Base64String `json:"prev_msg_id"`
	MsgKey    []byte            `json:"msg_key"`
	RecpKeys  []struct {
		Key    keys.Base64String `json:"key"`
		Scheme keys.KeyScheme    `json:"scheme"`
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
	feed, err := f.Feed()
	require.NoError(t, err)

	var m tfk.Message
	err = m.UnmarshalBinary(bt.Input.PrevMsgID)
	require.NoError(t, err)
	msg, err := m.Message()
	require.NoError(t, err)

	out, err := bxr.Encrypt(
		bt.Input.PlainText,
		feed,
		msg,
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
