// SPDX-License-Identifier: MIT

package migrations

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cryptix/go/logging/logtest"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/offset2"

	"go.cryptoscope.co/ssb/message/legacy"
	"go.cryptoscope.co/ssb/repo"
)

func TestUpgradeToMultiMessage(t *testing.T) {
	r := require.New(t)

	logger, _ := logtest.KitLogger(t.Name(), t)

	testPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(testPath)
	os.MkdirAll(testPath, 0700)
	testRepo := repo.New(testPath)

	// testlog is a link to plugins/gossip/testdata/largeRepo/log
	out, err := exec.Command("cp", "-rL", "testlog", testRepo.GetPath("log")).CombinedOutput()
	r.NoError(err, "copy failed:%s", string(out))

	from, err := offset2.Open(testRepo.GetPath("log"), msgpack.New(&legacy.OldStoredMessage{}))
	r.NoError(err)

	// check for legacy testdata from the gossip plugin
	currSeq, err := from.Seq().Value()
	r.NoError(err)
	r.EqualValues(431, currSeq)
	msgV, err := from.Get(margaret.BaseSeq(430))
	r.NoError(err)
	osm, ok := msgV.(legacy.OldStoredMessage)
	r.True(ok, "wrong type: %T", msgV)
	r.Equal("%OnSDT0rFoLnUVkp2VoCZ4bAmsTQiI8LbWUTdaE5j9KM=.sha256", osm.Key.Ref())
	r.NoError(from.Close())

	// do the dance
	did, err := UpgradeToMultiMessage(logger, testRepo)
	r.NoError(err)
	r.True(did)

	migratedLog, err := repo.OpenLog(testRepo)
	r.NoError(err)

	migratedSeq, err := migratedLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(431, migratedSeq)

	r.Equal(1, CurrentVersion(testRepo))
	did, err = UpgradeToMultiMessage(logger, testRepo)
	r.NoError(err)
	r.False(did)
}
