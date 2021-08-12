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

	"go.cryptoscope.co/ssb/private/keys"
	refs "go.mindeco.de/ssb-refs"
)

func TestInvalidFuzzed(t *testing.T) {
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
