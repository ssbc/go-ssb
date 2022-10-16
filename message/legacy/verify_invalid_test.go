// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/private/keys"
	refs "go.mindeco.de/ssb-refs"
)

// TODO: this somehow started panicing..?!
// --- FAIL: TestInvalidFuzzed (0.01s)
// panic: runtime error: index out of range [2] with length 1 [recovered]
// 	panic: runtime error: index out of range [2] with length 1

// goroutine 8 [running]:
// testing.tRunner.func1.2(0x4ab6b60, 0xc00012a120)
// 	/usr/local/Cellar/go/1.16.6/libexec/src/testing/testing.go:1143 +0x332
// testing.tRunner.func1(0xc000420300)
// 	/usr/local/Cellar/go/1.16.6/libexec/src/testing/testing.go:1146 +0x4b6
// panic(0x4ab6b60, 0xc00012a120)
// 	/usr/local/Cellar/go/1.16.6/libexec/src/runtime/panic.go:965 +0x1b9
// encoding/base64.(*Encoding).decodeQuantum(0xc0000b4000, 0xc00293aebf, 0x1, 0x1, 0xc002764d1c, 0x78, 0x84, 0x58, 0x5400108, 0x40, ...)
// 	/usr/local/Cellar/go/1.16.6/libexec/src/encoding/base64/base64.go:362 +0x545
// encoding/base64.(*Encoding).Decode(0xc0000b4000, 0xc00293ae80, 0x40, 0x40, 0xc002764d1c, 0x78, 0x84, 0xc0026eb5b8, 0xc0026eb670, 0x0)
// 	/usr/local/Cellar/go/1.16.6/libexec/src/encoding/base64/base64.go:530 +0x5d3
// go.cryptoscope.co/ssb/message/legacy.NewSignatureFromBase64(0xc002764d1c, 0x84, 0x84, 0x6cd, 0xc0026eb670, 0x0, 0x0, 0xc002764e00)
// 	/Users/cryptix/ssb/go-ssb/message/legacy/signature.go:58 +0x1a6
// go.cryptoscope.co/ssb/message/legacy.ExtractSignature(0xc002764700, 0x6a3, 0x6cd, 0x49cef01, 0xc00293f670, 0x0, 0x0, 0x6a3, 0x6cd, 0x0, ...)
// 	/Users/cryptix/ssb/go-ssb/message/legacy/signature.go:33 +0x10a
// go.cryptoscope.co/ssb/message/legacy.VerifyWithBuffer(0xc00023f500, 0x6c3, 0x700, 0x0, 0xc0028eb740, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
// 	/Users/cryptix/ssb/go-ssb/message/legacy/verify.go:115 +0x8b5
// go.cryptoscope.co/ssb/message/legacy.Verify(0xc00023f500, 0x6c3, 0x700, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, ...)
// 	/Users/cryptix/ssb/go-ssb/message/legacy/verify.go:44 +0x110
// go.cryptoscope.co/ssb/message/legacy.TestInvalidFuzzed(0xc000420300)
// 	/Users/cryptix/ssb/go-ssb/message/legacy/verify_invalid_test.go:46 +0x2a7
// testing.tRunner(0xc000420300, 0x4b5d080)
// 	/usr/local/Cellar/go/1.16.6/libexec/src/testing/testing.go:1193 +0xef
// created by testing.(*T).Run
// 	/usr/local/Cellar/go/1.16.6/libexec/src/testing/testing.go:1238 +0x2b3
// exit status 2
func TestInvalidFuzzed(t *testing.T) {
	if testutils.SkipOnCI(t) {
		// https://github.com/ssbc/go-ssb/pull/167
		return
	}

	r := require.New(t)
	a := assert.New(t)

	type state struct{}

	type testCase struct {
		State   *state
		HMACKey keys.Base64String `json:"hmacKey"`

		Message json.RawMessage

		Error *string
		Valid bool
		ID    refs.MessageRef
	}

	var rawMsgs []testCase

	d, err := ioutil.ReadFile("./testdata-fuzzed.json")
	r.NoError(err, "failed to read file")

	err = json.Unmarshal(d, &rawMsgs)
	r.NoError(err)

	for i, m := range rawMsgs {
		r, d, err := Verify(m.Message, nil)

		if m.Valid {
			if !a.NoError(err, "in msg %d", i) {
				continue
			}
			a.NotNil(r, i)
			a.NotNil(d, i)
		} else {
			if !a.Error(err, "in msg %d", i) {
				t.Logf("expected error: %s", *m.Error)
				continue
			}
			a.Zero(r, i)
			a.Zero(d, i)
		}
	}
}
