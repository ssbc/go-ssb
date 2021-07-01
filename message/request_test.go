package message

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamArgsLimitDefault(t *testing.T) {
	a := assert.New(t)

	type testCase struct {
		Input []byte
		Want  streamArgs
	}

	cases := []testCase{
		{
			Input: []byte(`{"limit": 23}`),
			Want:  streamArgs{Limit: 23},
		},

		{
			Input: []byte(`{}`),
			Want:  streamArgs{Limit: -1},
		},

		{
			Input: []byte(`{"reverse": true}`),
			Want:  streamArgs{Limit: -1, Reverse: true},
		},
	}

	for i, tc := range cases {
		got := NewStreamArgs()
		err := json.Unmarshal(tc.Input, &got)
		a.NoError(err, "decoding case %d failed", i+1)
		a.Equal(tc.Want, got, "wrong data for case %d", i+1)
	}

}
