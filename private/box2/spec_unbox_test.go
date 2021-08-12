// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/private/keys"
	"go.mindeco.de/ssb-refs/tfk"
)

type unboxSpecTest struct {
	genericSpecTest

	Input  unboxSpecTestInput  `json:"input"`
	Output unboxSpecTestOutput `json:"output"`
}

type unboxSpecTestInput struct {
	Ciphertext []byte            `json:"ciphertext"`
	FeedID     keys.Base64String `json:"feed_id"`
	PrevMsgID  keys.Base64String `json:"prev_msg_id"`
	Recipient  struct {
		Key    keys.Base64String `json:"key"`
		Scheme keys.KeyScheme    `json:"scheme"`
	} `json:"recipient"`
}

type unboxSpecTestOutput struct {
	Plaintext []byte `json:"plain_text"`
}

func (ut unboxSpecTest) Test(t *testing.T) {
	bxr := NewBoxer(nil)

	var f tfk.Feed
	err := f.UnmarshalBinary(ut.Input.FeedID)
	require.NoError(t, err)
	feed, err := f.Feed()
	require.NoError(t, err)

	var m tfk.Message
	err = m.UnmarshalBinary(ut.Input.PrevMsgID)
	require.NoError(t, err)
	msg, err := m.Message()
	require.NoError(t, err)

	out, err := bxr.Decrypt(
		ut.Input.Ciphertext,
		feed,
		msg,
		[]keys.Recipient{
			{Key: keys.Key(ut.Input.Recipient.Key), Scheme: ut.Input.Recipient.Scheme},
		},
	)

	require.NoError(t, err, "failed to decrypt")

	require.Equal(t, ut.Output.Plaintext, out)
}
