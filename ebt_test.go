// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"
)

func TestEBTNotes(t *testing.T) {
	a := assert.New(t)

	type tcase struct {
		Network    json.RawMessage
		WantedNote Note

		ExpectErr bool
	}

	var tcases = []tcase{
		{
			Network:    []byte("-2"),
			WantedNote: Note{},
			ExpectErr:  true,
		},

		{
			Network:    []byte("-1"),
			WantedNote: Note{Replicate: false, Receive: false, Seq: 0},
		},

		{
			Network:    []byte("22"),
			WantedNote: Note{Replicate: true, Receive: true, Seq: 11},
		},
		{
			Network:    []byte("23"),
			WantedNote: Note{Replicate: true, Receive: false, Seq: 11},
		},

		{
			Network:    []byte("24"),
			WantedNote: Note{Replicate: true, Receive: true, Seq: 12},
		},
	}

	for _, tc := range tcases {

		var n Note
		err := n.UnmarshalJSON(tc.Network)
		if tc.ExpectErr {
			a.NotNil(err, "expected an error for %s", string(tc.Network))
		} else {
			a.NoError(err, "expected no error for %s", string(tc.Network))
			a.Equal(tc.WantedNote, n)
		}
	}
}

func TestEBTFrontierMarshal(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	f1, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte("ab"), 16), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	f2, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte("ac"), 16), refs.RefAlgoFeedGabby)
	r.NoError(err)

	var example = NewNetworkFrontier()
	example.Frontier[f1.String()] = Note{Seq: 23, Replicate: true, Receive: true}
	example.Frontier[f2.String()] = Note{Seq: 42, Replicate: true, Receive: true}

	data, err := json.Marshal(example)
	r.NoError(err)
	a.EqualValues(`{"@YWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWI=.ed25519":46,"ssb:feed/gabbygrove-v1/YWNhY2FjYWNhY2FjYWNhY2FjYWNhY2FjYWNhY2FjYWM=":84}`, string(data))
}

func TestEBTFrontierUnmarshal(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	input := `{
	"@YWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWI=.ed25519":46,
	"ssb:feed/gabbygrove-v1/YWNhY2FjYWNhY2FjYWNhY2FjYWNhY2FjYWNhY2FjYWM=":84
}`

	f1, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte("ab"), 16), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	f2, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte("ac"), 16), refs.RefAlgoFeedGabby)
	r.NoError(err)

	var ex1 = NewNetworkFrontier()
	err = ex1.UnmarshalJSON([]byte(input))
	r.NoError(err)
	a.Len(ex1.Frontier, 2)

	note, has := ex1.Frontier[f1.String()]
	r.True(has)
	a.Equal(note, Note{Seq: 23, Replicate: true, Receive: true})

	note, has = ex1.Frontier[f2.String()]
	r.True(has)
	a.Equal(note, Note{Seq: 42, Replicate: true, Receive: true})
}
