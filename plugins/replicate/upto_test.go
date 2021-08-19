// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package replicate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mindeco.de/log"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestListing(t *testing.T) {
	// <boilerplate>
	r := require.New(t)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(tRepoPath)
	// </boilerplate>

	// make three new keypairs with nicknames
	n2kp := make(map[string]ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", refs.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", refs.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kpCloe, err := repo.NewKeyPair(tRepo, "cloe", refs.RefAlgoFeedBendyButt)
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
		sbot.DisableNetworkNode(),
	)
	r.NoError(err)

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", refs.NewAboutName(kpArny.ID(), "i'm arny!")},
		{"bert", refs.NewAboutName(kpBert.ID(), "i'm bert!")},
	}
	for idx, intro := range intros {
		_, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)

		for i := 0; i < 10; i++ {
			_, err := mainbot.PublishAs(intro.as, refs.NewPost(fmt.Sprint(i)))
			r.NoError(err, "publish %d failed", idx)
		}
	}
	r.EqualValues(len(intros)*11-1, mainbot.ReceiveLog.Seq(), "has the expected messages")

	assertListing := func(wanted int) {
		set := mainbot.Lister().ReplicationList()
		feeds, err := set.List()
		r.NoError(err)

		respSet, err := ssb.WantedFeedsWithSeqs(mainbot.Users, feeds)
		r.NoError(err)

		assert.Len(t, respSet, wanted)
	}

	// check with no on one on the want list
	assertListing(0)

	// add a feed and check again
	mainbot.Replicate(kpArny.ID())
	assertListing(1)

	// add another feed and check again
	mainbot.Replicate(kpBert.ID())
	assertListing(2)

	// now add cloe, which has no message yet (should be listed as zero)
	mainbot.Replicate(kpCloe.ID())
	assertListing(3)

	// cleanup
	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}
