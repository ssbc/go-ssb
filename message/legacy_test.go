package message

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyNays(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)
	for _, folder := range []string{"duplicate", "number", "surrogate", "syntax"} {

		walk := func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// t.Log("testing", path)
			data, err := ioutil.ReadFile(path)
			r.NoError(err)

			var v interface{}
			err = json.Unmarshal(data, &v)
			r.Error(err, "stdjson decoded it: %+v", v)

			d, err := EncodePreserveOrder(data)
			a.Equal("", string(d), "message %s produced output", path)
			a.Error(err, "message %s did not produce an error", path)
			return nil
		}
		err := filepath.Walk(filepath.Join("legacy-value-testdata", folder), walk)
		r.NoError(err)
	}
}

func TestLegacyYays(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)
	walk := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != "" {
			// we are interested only in the input files and will open the others after encode
			return nil
		}

		data, err := ioutil.ReadFile(path)
		r.NoError(err)

		var v interface{}
		err = json.Unmarshal(data, &v)
		r.NoError(err, "stdjson did not decode it: %+v", v)

		d, err := EncodePreserveOrder(data)
		a.NotEqual("", string(d), "message %s produced no output", path)
		a.NoError(err, "message %s produced an error", path)

		return nil
	}
	err := filepath.Walk("legacy-value-testdata/yay", walk)
	r.NoError(err)
}
