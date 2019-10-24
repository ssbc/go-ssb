// SPDX-License-Identifier: MIT

package names

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/plugins2"
	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/sbot"
)

func XTestNames(t *testing.T) {
	defer leakcheck.Check(t)
	r := require.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(tRepoPath)

	// make three new keypairs with nicknames
	n2kp := make(map[string]*ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", ssb.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kpCloe, err := repo.NewKeyPair(tRepo, "cloe", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["cloe"] = kpCloe

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 3)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	mainbot, err := sbot.New(
		sbot.WithInfo(logger),
		sbot.WithRepoPath(tRepoPath),
		sbot.WithHMACSigning(hk),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.MountPlugin(&Plugin{}, plugins2.AuthMaster)),
		sbot.WithUNIXSocket(),
	)
	r.NoError(err)

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", ssb.NewAboutName(kpArny.Id, "i'm arny!")},
		{"bert", ssb.NewAboutName(kpBert.Id, "i'm bert!")},
		{"bert", ssb.NewAboutName(kpCloe.Id, "that cloe")},
		{"cloe", ssb.NewAboutName(kpBert.Id, "iditot")},
		{"cloe", ssb.NewAboutName(kpCloe.Id, "i'm cloe!")},
	}

	for idx, intro := range intros {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
	}

	// assert helper
	checkLogSeq := func(l margaret.Log, seq int) {
		v, err := l.Seq().Value()
		r.NoError(err)
		r.EqualValues(seq, v.(margaret.Seq).Seq())
	}

	checkLogSeq(mainbot.RootLog, len(intros)-1) // got all the messages

	c, err := client.NewUnix(context.TODO(), filepath.Join(tRepoPath, "socket"))
	r.NoError(err)

	all, err := c.NamesGet()
	r.NoError(err)

	want := map[string]string{
		"arny": "i'm arny!",
		"bert": "i'm bert!",
		"cloe": "i'm cloe!",
	}

	for who, wantName := range want {
		name, ok := all.GetCommonName(n2kp[who].Id)
		r.True(ok)
		r.Equal(wantName, name)
	}

	for who, wantName := range want {
		name2, err := c.NamesSignifier(*n2kp[who].Id)
		r.NoError(err)
		r.Equal(wantName, name2)
	}

	r.NoError(c.Close())

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}
