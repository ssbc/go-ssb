// SPDX-License-Identifier: MIT

package sbot

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/private"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"

	"github.com/go-kit/kit/log"
	"go.cryptoscope.co/ssb/repo"

	"github.com/stretchr/testify/require"
)

func TestMultipleIdentities(t *testing.T) {
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
	mainbot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		WithHMACSigning(hk),
		LateOption(MountSimpleIndex("get", indexes.OpenGet)),
		DisableNetworkNode(),
	)
	r.NoError(err)

	// boxing helper
	box := func(v interface{}, recpts ...*ssb.FeedRef) []byte {
		msg, err := json.Marshal(v)
		r.NoError(err, "failed to marshal privmsg")
		ciph, err := private.Box(msg, recpts...)
		r.NoError(err, "failed to box privmg")
		return ciph
	}

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", ssb.NewContactFollow(kpBert.Id)},
		{"bert", ssb.NewContactFollow(kpArny.Id)},
		{"bert", ssb.NewContactFollow(kpCloe.Id)},
		{"cloe", ssb.NewContactFollow(kpArny.Id)},
		{"arny", map[string]interface{}{"hello": 123}},
		{"bert", map[string]interface{}{"world": 456}},
		{"cloe", map[string]interface{}{"test": 789}},
		{"arny", box("A: just talking to myself", kpArny.Id)},
		{"bert", box("B: just talking to myself", kpBert.Id)},
		{"cloe", box("C: just talking to myself", kpCloe.Id)},
		{"cloe", box("hellooo", kpBert.Id, kpCloe.Id)},
		{"bert", box("you toooo", kpBert.Id, kpCloe.Id)},
		{"arny", box("to the others", kpBert.Id, kpCloe.Id)},
	}

	for idx, intro := range intros {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
		msg, err := mainbot.Get(*ref)
		r.NoError(err)
		r.NotNil(msg)

		r.True(msg.Author().Equal(n2kp[intro.as].Id))
	}

	// assert helper
	checkLogSeq := func(l margaret.Log, seq int) {
		v, err := l.Seq().Value()
		r.NoError(err)
		r.EqualValues(seq, v.(margaret.Seq).Seq())
	}

	checkLogSeq(mainbot.RootLog, len(intros)-1) // got all the messages

	// individual PMs got delivered
	pl, ok := mainbot.GetMultiLog("privLogs")
	r.True(ok, "no privLogs")

	arnies, err := pl.Get(kpArny.Id.StoredAddr())
	r.NoError(err)
	berts, err := pl.Get(kpBert.Id.StoredAddr())
	r.NoError(err)
	cloes, err := pl.Get(kpCloe.Id.StoredAddr())
	r.NoError(err)

	// 0 indexed
	checkLogSeq(arnies, 0) // just to her self
	checkLogSeq(berts, 3)  // self + hello + reply + from arny
	checkLogSeq(cloes, 3)  // self + hello + reply + from arny

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}
