// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package box2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/internal/testutils"
)

func seq(start, end int) []byte {
	out := make([]byte, end-start)
	for i := range out {
		out[i] = byte(i + start)
	}
	return out
}

func TestSpec(t *testing.T) {
	if testutils.SkipOnCI(t) {
		return
	}

	dir, err := os.Open(filepath.Join("spec", "vectors"))
	if !assert.NoError(t, err, "open vectors dir") {
		t.Log("suggestion: run 'git clone https://github.com/ssbc/envelope-spec spec'")
		t.Log("or clone it somewhere else and create a symlink to it here named 'spec'")
		t.FailNow()
	}

	ls, err := dir.Readdir(0)
	require.NoError(t, err, "list vectors dir")

	for _, fi := range ls {
		if !strings.HasSuffix(fi.Name(), ".json") {
			continue
		}

		f, err := os.Open(filepath.Join("spec", "vectors", fi.Name()))
		require.NoError(t, err, "open vector json file")

		var spec genericSpecTest
		err = json.NewDecoder(f).Decode(&spec)
		require.NoError(t, err, "json parse error")

		f.Seek(0, 0)

		testName := fmt.Sprintf("%s/%s", spec.Type, filepath.Base(f.Name()))
		switch spec.Type {
		case "box":
			var spec boxSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(testName, spec.Test)
		case "unslot":
			var spec unslotSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(testName, spec.Test)
		case "unbox":
			var spec unboxSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(testName, spec.Test)
		case "derive_secret":
			var spec deriveSecretSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(testName, spec.Test)
		case "cloaked_id":
			var spec cloakedIDSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(testName, spec.Test)
		default:
			t.Logf("no test code for type %q", spec.Type)
			t.Log("skipping:", spec.Description)
		}
	}
}

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
