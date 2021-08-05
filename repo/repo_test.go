// SPDX-License-Identifier: MIT

package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/margaret"
)

func TestNew(t *testing.T) {
	r := require.New(t)

	rpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(rpath)

	repo := New(rpath)

	rl, err := OpenLog(repo)
	r.NoError(err, "failed to open root log")
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq)

	if !t.Failed() {
		os.RemoveAll(rpath)
	}
}
