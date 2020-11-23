package sbot

import (
	"encoding/json"
	"testing"
)

func TestManifest(t *testing.T) {
	var manifestObj map[string]interface{}
	err := json.Unmarshal(json.RawMessage(manifestBlob), &manifestObj)
	if err != nil {
		t.Error(err)
	}
}
