// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	r := require.New(t)

	rpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(rpath)

	repo := New(rpath)

	rl, err := OpenLog(repo)
	r.NoError(err, "failed to open root log")

	r.EqualValues(-1, rl.Seq())

	if !t.Failed() {
		os.RemoveAll(rpath)
	}
}
