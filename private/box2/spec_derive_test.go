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

type deriveSecretSpecTest struct {
	genericSpecTest

	Input  deriveSecretSpecTestInput  `json:"input"`
	Output deriveSecretSpecTestOutput `json:"output"`
}

type deriveSecretSpecTestInput struct {
	FeedID    keys.Base64String `json:"feed_id"`
	PrevMsgID keys.Base64String `json:"prev_msg_id"`
	MsgKey    []byte            `json:"msg_key"`
}

type deriveSecretSpecTestOutput struct {
	ReadKey   []byte `json:"read_key"`
	HeaderKey []byte `json:"header_key"`
	BodyKey   []byte `json:"body_key"`
}

func (dt deriveSecretSpecTest) Test(t *testing.T) {
	var f tfk.Feed
	err := f.UnmarshalBinary(dt.Input.FeedID)
	require.NoError(t, err)
	feed, err := f.Feed()
	require.NoError(t, err)

	var m tfk.Message
	err = m.UnmarshalBinary(dt.Input.PrevMsgID)
	require.NoError(t, err)
	msg, err := m.Message()
	require.NoError(t, err)

	info, err := makeInfo(feed, msg)
	require.NoError(t, err)

	var readKey, headerKey, bodyKey [32]byte

	DeriveTo(readKey[:], dt.Input.MsgKey, info([]byte("read_key"))...)
	DeriveTo(headerKey[:], readKey[:], info([]byte("header_key"))...)
	DeriveTo(bodyKey[:], readKey[:], info([]byte("body_key"))...)

	require.EqualValues(t, dt.Output.ReadKey, readKey[:], "read key wrong")
	require.EqualValues(t, dt.Output.HeaderKey, headerKey[:], "header wrong")
	require.EqualValues(t, dt.Output.BodyKey, bodyKey[:], "body wrong")
}
