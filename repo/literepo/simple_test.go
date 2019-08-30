package literepo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/message/multimsg"
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
		sbot.WithRootLog(multimsg.NewWrappedLog(sqlitelog)),
		sbot.MountMultiLog("userFeeds", sqlitelog.UserFeeds()),
	)
	r.NoError(err)

	for i := 0; i < 10; i++ {

		seq, err := bot.PublishLog.Append(struct {
			A int
			S string
		}{i, "23"})
		r.NoError(err)
		r.Equal(int64(i), seq.Seq())
	}

	bot.Shutdown()

	r.NoError(bot.Close())
}
