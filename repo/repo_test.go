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

	repo, err := New(rpath)
	r.NoError(err, "failed to create repo")

	kf, err := repo.KnownFeeds()
	r.NoError(err, "failed to query known feeds")
	r.Len(kf, 0, "new repo should be empty")

	l := repo.Log()
	seq, err := l.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.Seq(-1), seq)

	r.NoError(repo.Close(), "failed to close repo")

	if !t.Failed() {
		os.RemoveAll(rpath)
	}
}
