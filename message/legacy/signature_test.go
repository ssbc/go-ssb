// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
)

func TestSignatureVerify(t *testing.T) {
	a, r := assert.New(t), require.New(t)
	n := len(testMessages)
	if testing.Short() {
		n = min(50, n)
	}
	for i := 1; i < n; i++ {
		enc, err := PrettyPrint(testMessages[i].Input)
		r.NoError(err, "encode failed")

		msgWOsig, sig, err := ExtractSignature(enc)
		r.NoError(err, "extractSig failed")
		a.Equal(testMessages[i].NoSig, msgWOsig)

		err = sig.Verify(msgWOsig, testMessages[i].Author)
		r.NoError(err, "verify failed")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestCompatHMACVerify(t *testing.T) {
	r := require.New(t)
	seed := makeRandBytes(t, 32)

	kp, err := ssb.NewKeyPair(bytes.NewReader(seed), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	hmacKey := makeRandBytes(t, 32)
	var hk [32]byte
	copy(hk[:], hmacKey)

	// TODO: be more creative with test data
	var lm LegacyMessage
	lm.Hash = "sha256"
	lm.Author = kp.ID().String()
	lm.Content = map[string]interface{}{
		"type":  "test",
		"hello": "world",
	}

	mr, msgbytes, err := lm.Sign(kp.Secret(), &hk)
	r.NoError(err)
	r.NotNil(mr)

	env := []string{
		"testaction=hmac_verify",
		"testhmackey=" + base64.StdEncoding.EncodeToString(hmacKey),
		"testseed=" + base64.StdEncoding.EncodeToString(seed),
		"testpublic=" + kp.ID().String(),
		"testobj=" + base64.StdEncoding.EncodeToString(msgbytes),
	}
	runCompatScript(t, env)

	_, _, err = Verify(msgbytes, &hk)
	r.NoError(err)
}

func TestCompatHMACSign(t *testing.T) {
	r := require.New(t)
	seed := makeRandBytes(t, 32)

	kp, err := ssb.NewKeyPair(bytes.NewReader(seed), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	hmacKey := makeRandBytes(t, 32)
	var hk [32]byte
	copy(hk[:], hmacKey)

	// TODO: be more creative with test data
	var lm LegacyMessage
	lm.Hash = "sha256"
	lm.Author = kp.ID().String()
	lm.Content = map[string]interface{}{
		"type":  "test",
		"hello": "world",
	}

	mr, msgbytes, err := lm.Sign(kp.Secret(), &hk)
	r.NoError(err)
	r.NotNil(mr)

	_, _, err = Verify(msgbytes, &hk)
	r.NoError(err)

	// this is a bit dull but used for comparing the output from js
	// extract _just_ the signature back from the msg
	var justTheSig struct {
		Sig string `json:"signature"`
	}
	err = json.Unmarshal(msgbytes, &justTheSig)
	r.NoError(err)

	// without the sig for js
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(lm)
	r.NoError(err)

	pp, err := PrettyPrint(buf.Bytes())
	r.NoError(err)

	env := []string{
		"testaction=hmac_sign",
		"testhmackey=" + base64.StdEncoding.EncodeToString(hmacKey),
		"testseed=" + base64.StdEncoding.EncodeToString(seed),
		"testpublic=" + kp.ID().String(),
		"testobj=" + base64.StdEncoding.EncodeToString(pp),
	}
	out := runCompatScript(t, env)
	r.Equal(justTheSig.Sig, strings.TrimSpace(out))

}

func TestCompatVerify(t *testing.T) {
	r := require.New(t)
	seed := makeRandBytes(t, 32)

	kp, err := ssb.NewKeyPair(bytes.NewReader(seed), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// TODO: be more creative with test data
	var lm LegacyMessage
	lm.Hash = "sha256"
	lm.Author = kp.ID().String()
	lm.Content = map[string]interface{}{
		"type":  "test",
		"hello": "world",
	}

	mr, msgbytes, err := lm.Sign(kp.Secret(), nil)
	r.NoError(err)
	r.NotNil(mr)

	_, _, err = Verify(msgbytes, nil)
	r.NoError(err)

	env := []string{
		"testaction=verify",
		"testseed=" + base64.StdEncoding.EncodeToString(seed),
		"testpublic=" + kp.ID().String(),
		"testobj=" + base64.StdEncoding.EncodeToString(msgbytes),
	}
	runCompatScript(t, env)
}

func TestCompatSignature(t *testing.T) {
	r := require.New(t)
	seed := makeRandBytes(t, 32)

	kp, err := ssb.NewKeyPair(bytes.NewReader(seed), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// TODO: be more creative with test data
	var lm LegacyMessage
	lm.Hash = "sha256"
	lm.Author = kp.ID().String()
	lm.Content = map[string]interface{}{
		"type":  "test",
		"hello": "world",
	}

	mr, msgbytes, err := lm.Sign(kp.Secret(), nil)
	r.NoError(err)
	r.NotNil(mr)

	_, _, err = Verify(msgbytes, nil)
	r.NoError(err)

	// this is a bit dull but used for comparing the output from js
	// extract _just_ the signature back from the msg
	var justTheSig struct {
		Sig string `json:"signature"`
	}
	err = json.Unmarshal(msgbytes, &justTheSig)
	r.NoError(err)

	// without the sig for js
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(lm)
	r.NoError(err)

	pp, err := PrettyPrint(buf.Bytes())
	r.NoError(err)

	env := []string{
		"testaction=sign",
		"testseed=" + base64.StdEncoding.EncodeToString(seed),
		"testpublic=" + kp.ID().String(),
		"testobj=" + base64.StdEncoding.EncodeToString(pp),
	}
	out := runCompatScript(t, env)
	r.Equal(justTheSig.Sig, strings.TrimSpace(out))
}

func runCompatScript(t *testing.T, env []string) string {
	r := require.New(t)

	var buf bytes.Buffer
	cmd := exec.Command("node", "./signature_compat.js")
	cmd.Stderr = os.Stderr
	cmd.Stdout = &buf
	cmd.Env = env

	err := cmd.Run()
	r.NoError(err)

	return buf.String()
}

func makeRandBytes(t *testing.T, n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err, "failed to make %d randbytes", n)
	return b
}
