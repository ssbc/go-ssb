// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/private/keys"
)

type cloakedIDSpecTest struct {
	genericSpecTest

	Input  cloakedIDSpecTestInput  `json:"input"`
	Output cloakedIDSpecTestOutput `json:"output"`
}

type cloakedIDSpecTestInput struct {
	MessageID keys.Base64String `json:"public_Msg_id"`
	ReadKey   keys.Base64String `json:"read_key"`
}

type cloakedIDSpecTestOutput struct {
	CloakedID keys.Base64String `json:"cloaked_msg_id"`
}

func (ct cloakedIDSpecTest) Test(t *testing.T) {
	// TOOD: one-off use of dervieTo() should probably be an (exported?) function
	cloaked := make([]byte, 32)
	DeriveTo(cloaked, ct.Input.ReadKey, []byte("cloaked_msg_id"), ct.Input.MessageID)
	require.EqualValues(t, ct.Output.CloakedID, cloaked, "wrong cloacked ID")
}
