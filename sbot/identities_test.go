// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/private/box"
	"go.cryptoscope.co/ssb/repo"
)

// TODO: refactor for multi manager
func XTestMultipleIdentities(t *testing.T) {
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

	kpsByPath, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kpsByPath, 3)

	var kps []ssb.KeyPair
	for _, v := range kpsByPath {
		kps = append(kps, v)
	}

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	mainbot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		WithHMACSigning(hk),
		DisableNetworkNode(),
	)
	r.NoError(err)

	// boxing helper
	b := box.NewBoxer(nil)
	box := func(post string, recpts ...refs.FeedRef) []byte {
		msg, err := json.Marshal(refs.NewPost(post))
		r.NoError(err, "failed to marshal privmsg")

		ciph, err := b.Encrypt(msg, recpts...)
		r.NoError(err, "failed to box privmg")
		return ciph
	}

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", refs.NewContactFollow(kpBert.ID())},
		{"bert", refs.NewContactFollow(kpArny.ID())},
		{"bert", refs.NewContactFollow(kpCloe.ID())},
		{"cloe", refs.NewContactFollow(kpArny.ID())},
		{"arny", map[string]interface{}{"hello": 123}},
		{"bert", map[string]interface{}{"world": 456}},
		{"cloe", map[string]interface{}{"test": 789}},
		{"arny", box("A: just talking to myself", kpArny.ID())},
		{"bert", box("B: just talking to myself", kpBert.ID())},
		{"cloe", box("C: just talking to myself", kpCloe.ID())},
		{"cloe", box("hellooo", kpBert.ID(), kpCloe.ID())},
		{"bert", box("you toooo", kpBert.ID(), kpCloe.ID())},
		{"arny", box("to the others", kpBert.ID(), kpCloe.ID())},
	}

	for idx, intro := range intros {
		msg, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)

		r.True(msg.Author().Equal(n2kp[intro.as].ID()))
	}

	// assert helper
	checkLogSeq := func(l margaret.Log, seq int) {
		r.EqualValues(seq, l.Seq())
	}

	checkLogSeq(mainbot.ReceiveLog, len(intros)-1) // got all the messages

	src, err := mainbot.ReceiveLog.Query()
	r.NoError(err)

	ctx := context.Background()
	for {
		v, err := src.Next(ctx)
		if luigi.IsEOS(err) {
			break
		}
		r.NoError(err)
		msg, ok := v.(refs.Message)
		r.True(ok, "wrong type: %T", v)
		r.NotNil(msg)

		var emptyv interface{}
		err = json.Unmarshal(msg.ValueContentJSON(), &emptyv)
		r.NoError(err)
		// spew.Dump(emptyv)
	}

	// individual PMs got delivered
	pl, ok := mainbot.GetMultiLog("privLogs")
	r.True(ok, "no privLogs")

	arnies, err := pl.Get(storedrefs.Feed(kpArny.ID()))
	r.NoError(err)
	berts, err := pl.Get(storedrefs.Feed(kpBert.ID()))
	r.NoError(err)
	cloes, err := pl.Get(storedrefs.Feed(kpCloe.ID()))
	r.NoError(err)

	// 0 indexed
	checkLogSeq(arnies, 0) // just to her self
	checkLogSeq(berts, 3)  // self + hello + reply + from arny
	checkLogSeq(cloes, 3)  // self + hello + reply + from arny

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}
