// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package private

import (
	"encoding/hex"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupsInfoSort(t *testing.T) {
	a := assert.New(t)

	foo := []byte("foo")
	bar := []byte("bar")
	baz := []byte("baz")
	aaa := []byte("aaa")

	infos := [][]byte{foo, bar, baz, aaa}
	a.Equal(foo, infos[0])
	sort.Sort(bytesSlice(infos))

	a.Equal(aaa, infos[0])
	a.Equal(bar, infos[1])
	a.Equal(baz, infos[2])
	a.Equal(foo, infos[3])

	one := []byte{0, 0, 0, 0, 1}
	two := []byte{0, 0, 0, 0, 2}

	thr := []byte{0, 0, 0, 0, 3}

	check := func(infos [][]byte) {
		a.Equal(one, infos[0])
		a.Equal(two, infos[1])
		a.Equal(thr, infos[2])
	}

	infos2 := [][]byte{
		one,
		two,
		thr,
	}

	sort.Sort(bytesSlice(infos2))
	check(infos2)

	infos3 := [][]byte{
		thr,
		two,
		one,
	}

	sort.Sort(bytesSlice(infos3))
	check(infos3)

	infos4 := [][]byte{
		thr,
		one,
		two,
	}

	sort.Sort(bytesSlice(infos4))
	check(infos4)
}

func TestGroupsInfoMeh(t *testing.T) {
	r := require.New(t)

	b1, err := hex.DecodeString("4c47c6a49310d86a78cac726356854dbd00728a4bfce96bbf8a0ac8a3d3b575b")
	r.NoError(err)

	b2, err := hex.DecodeString("272af1160d3306676bebd858aff5f8c4a1a4c7c52eeafd4693d8ce9566644ac8")
	r.NoError(err)

	sort1 := bytesSlice{b1, b2}
	sort.Sort(sort1)
	r.Equal(sort1[0], b2)

	sort2 := bytesSlice{b2, b1}
	sort.Sort(sort2)
	r.Equal(sort2[0], b2)
}
