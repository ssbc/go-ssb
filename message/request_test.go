// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

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
		Want  StreamArgs
	}

	cases := []testCase{
		{
			Input: []byte(`{"limit": 23}`),
			Want:  StreamArgs{Limit: 23},
		},

		{
			Input: []byte(`{}`),
			Want:  StreamArgs{Limit: -1},
		},

		{
			Input: []byte(`{"reverse": true}`),
			Want:  StreamArgs{Limit: -1, Reverse: true},
		},
	}

	for i, tc := range cases {
		got := NewStreamArgs()
		err := json.Unmarshal(tc.Input, &got)
		a.NoError(err, "decoding case %d failed", i+1)
		a.Equal(tc.Want, got, "wrong data for case %d", i+1)
	}

}

func TestRoundedInteger(t *testing.T) {
	a := assert.New(t)

	type testCase struct {
		Input []byte
		Want  RoundedInteger
	}

	cases := []testCase{
		{
			Input: []byte(`123`),
			Want:  123,
		},

		{
			Input: []byte(`1.234`),
			Want:  2,
		},
	}

	for i, tc := range cases {
		var got RoundedInteger
		err := json.Unmarshal(tc.Input, &got)
		a.NoError(err, "decoding case %d failed", i+1)
		a.Equal(tc.Want, got, "wrong data for case %d", i+1)
	}

}
