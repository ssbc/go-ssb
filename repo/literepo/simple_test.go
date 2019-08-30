package literepo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/repo/literepo"
	"go.cryptoscope.co/ssb/sbot"
)

func TestConstructBot(t *testing.T) {

	r := require.New(t)

	tpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tpath)

	sqlitelog, err := literepo.Open(tpath)
	r.NoError(err)

	bot, err := sbot.New(
		sbot.WithRootLog(sqlitelog),
	)
	r.NoError(err)

	seq, err := bot.PublishLog.Append(struct {
		A int
		S string
	}{1, "23"})
	r.NoError(err)
	r.NotEqual(int64(0), seq.Seq())

	bot.Shutdown()

	r.NoError(bot.Close())
}
