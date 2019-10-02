package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type cloakedIDSpecTest struct {
	genericSpecTest

	Input  cloakedIDSpecTestInput  `json:"input"`
	Output cloakedIDSpecTestOutput `json:"output"`
}

type cloakedIDSpecTestInput struct {
	MessageID b64str `json:"public_Msg_id"`
	ReadKey   b64str `json:"read_key"`
}

type cloakedIDSpecTestOutput struct {
	CloakedID b64str `json:"cloaked_msg_id"`
}

func (ct cloakedIDSpecTest) Test(t *testing.T) {
	// TOOD: one-off use of dervieTo() should probably be an (exported?) function
	cloaked := make([]byte, 32)
	deriveTo(cloaked, ct.Input.ReadKey, []byte("cloaked_msg_id"), ct.Input.MessageID)

	require.EqualValues(t, ct.Output.CloakedID, cloaked, "wrong cloacked ID")
}
