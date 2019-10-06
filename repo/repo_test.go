// SPDX-License-Identifier: MIT

package repo

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/margaret"
)

func TestNew(t *testing.T) {
	r := require.New(t)

	rpath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	repo := New(rpath)

	_, err = DefaultKeyPair(repo)
	r.NoError(err, "failed to open key pair")

	rl, err := OpenLog(repo)
	r.NoError(err, "failed to open root log")
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq)

	if !t.Failed() {
		os.RemoveAll(rpath)
	}
}
