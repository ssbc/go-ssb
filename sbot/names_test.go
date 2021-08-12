// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.mindeco.de/log"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

func TestNames(t *testing.T) {
	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
		// TODO: expose index flushing
	}

	defer leakcheck.Check(t)
	r := require.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(tRepoPath)

	// make three new keypairs with nicknames
	n2kp := make(map[string]ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", refs.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", refs.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kpCloe, err := repo.NewKeyPair(tRepo, "cloe", refs.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["cloe"] = kpCloe

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 3)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	mainbot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		WithHMACSigning(hk),
		WithListenAddr(":0"),
		LateOption(WithUNIXSocket()),
	)
	r.NoError(err)

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", refs.NewAboutName(kpArny.ID(), "i'm arny!")},
		{"bert", refs.NewAboutName(kpBert.ID(), "i'm bert!")},
		{"bert", refs.NewAboutName(kpCloe.ID(), "that cloe")},
		{"cloe", refs.NewAboutName(kpBert.ID(), "iditot")},
		{"cloe", refs.NewAboutName(kpCloe.ID(), "i'm cloe!")},
	}

	for idx, intro := range intros {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
	}

	// assert helper
	checkLogSeq := func(l margaret.Log, seq int) {
		r.EqualValues(seq, l.Seq())
	}

	checkLogSeq(mainbot.ReceiveLog, len(intros)-1) // got all the messages

	// TODO: flush indexes

	c, err := client.NewUnix(filepath.Join(tRepoPath, "socket"))
	r.NoError(err)

	all, err := c.NamesGet()
	r.NoError(err)

	want := map[string]string{
		"arny": "i'm arny!",
		"bert": "i'm bert!",
		"cloe": "i'm cloe!",
	}

	r.Len(all, len(want), "expected entries for all three keypairs")

	for who, wantName := range want {
		name, ok := all.GetCommonName(n2kp[who].ID())
		r.True(ok, "did not get a name for %s", who)
		r.Equal(wantName, name, "did not the right name for %s", who)
	}

	for who, wantName := range want {
		name2, err := c.NamesSignifier(n2kp[who].ID())
		r.NoError(err, "did not get a name for %s", who)
		r.Equal(wantName, name2, "did not the right name for %s", who)
	}

	r.NoError(c.Close())

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}
