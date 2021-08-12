// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package storedrefs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	refs "go.mindeco.de/ssb-refs"
)

func TestTangles(t *testing.T) {
	hash := []byte("1234567890abcdefghijklmnopqrstuv")
	wantv1 := append([]byte("v1:"), hash...)

	msgRef, err := refs.NewMessageRefFromBytes(hash, refs.RefAlgoBlobSSB1)
	if err != nil {
		t.Fatal(err)
	}

	gotv1 := TangleV1(msgRef)
	assert.Equal(t, wantv1, []byte(gotv1))

	// v2 tangles with names
	wantv2 := append([]byte("v2:fooo:"), hash...)
	gotv2 := TangleV2("fooo", msgRef)
	assert.Equal(t, wantv2, []byte(gotv2))
}
