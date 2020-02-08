package box2

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
)

type specTestHeader struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type genericSpecTest struct {
	specTestHeader

	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	ErrorCode interface{}     `json:"error_code"`
}

type boxSpecTest struct {
	genericSpecTest

	Input  boxSpecTestInput  `json:"input"`
	Output boxSpecTestOutput `json:"output"`
}

type boxSpecTestInput struct {
	PlainText     []byte          `json:"plain_text"`
	ExternalNonce []byte          `json:"external_nonce"`
	FeedID        *ssb.FeedRef    `json:"feed_id"`
	PrevMsgID     *ssb.MessageRef `json:"prev_msg_id"`
	MsgKey        []byte          `json:"msg_key"`
	RecpKeys      [][]byte        `json:"recp_keys"`
}

type boxSpecTestOutput struct {
	Ciphertext []byte `json:"ciphertext"`
	ErrorCode  string
}

func (bt boxSpecTest) Test(t *testing.T) {
	spew.Dump(bt)
	rand := bytes.NewBuffer([]byte(bt.Input.MsgKey))
	bxr := NewBoxer(rand)

	out, _ := bxr.Encrypt(
		nil,
		bt.Input.PlainText,
		bt.Input.FeedID,
		bt.Input.PrevMsgID,
		func() []keys.Key {
			out := make([]keys.Key, len(bt.Input.RecpKeys))
			for i := range out {
				out[i] = keys.Key(bt.Input.RecpKeys[i])
			}
			return out
		}(),
	)

	require.Equal(t, bt.Output.Ciphertext, out)
}

/*

type testable interface {
	Test(t *testing.T)
}

type parseTestFunc func(genericSpecTest) (testable, error)

func boxTestParse(st genericSpecTest) (testable, error) {
	var bt = boxSpecTest{
		genericSpecTest: st,
	}

	err := json.Unmarshal(st.Input, &bt.Input)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(st.Output, &bt.Output)
	if err != nil {
		return nil, err
	}

	return bt, nil
}


*/

func TestSpec(t *testing.T) {
	dir, err := os.Open(filepath.Join("spec", "vectors"))
	require.NoError(t, err, "open vectors dir")

	ls, err := dir.Readdir(0)
	require.NoError(t, err, "list vectors dir")

	for _, fi := range ls {
		if !strings.HasSuffix(fi.Name(), ".json") {
			continue
		}

		if !strings.HasPrefix(fi.Name(), "box") {
			continue
		}

		var spec boxSpecTest

		f, err := os.Open(filepath.Join("spec", "vectors", fi.Name()))
		require.NoError(t, err, "open vector json file")

		t.Log(f.Name())

		err = json.NewDecoder(f).Decode(&spec)
		require.NoError(t, err, "json parse error")

		if spec.Type != "box" {
			continue
		}

		t.Run(spec.Description, spec.Test)
	}
}
