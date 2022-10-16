// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy_test

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/legacy"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestSignMetafeedAnnouncment(t *testing.T) {
	r := require.New(t)

	var hmacSecret [32]byte
	rand.Read(hmacSecret[:])

	theUpgradingOne, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	theMeta, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedBendyButt)
	r.NoError(err)

	ma := legacy.NewMetafeedAnnounce(theMeta.ID(), theUpgradingOne.ID())

	signedMsg, err := ma.Sign(theMeta.Secret(), &hmacSecret)
	r.NoError(err)

	_, ok := legacy.VerifyMetafeedAnnounce(signedMsg, theUpgradingOne.ID(), &hmacSecret)
	r.True(ok, "verify failed")
}

func TestPublishMetafeedAnnounce(t *testing.T) {
	r := require.New(t)

	botPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(botPath)

	bot, err := sbot.New(
		sbot.WithMetaFeedMode(true),
		sbot.WithRepoPath(botPath),
	)
	r.NoError(err)

	subfeed, err := bot.MetaFeeds.CreateSubFeed(bot.KeyPair.ID(), "testfeed", refs.RefAlgoFeedSSB1)
	r.NoError(err)

	ma := legacy.NewMetafeedAnnounce(bot.KeyPair.ID(), subfeed)

	signedMsg, err := ma.Sign(bot.KeyPair.Secret(), nil)
	r.NoError(err)

	ref, err := bot.MetaFeeds.Publish(subfeed, signedMsg)
	r.NoError(err)

	msg, err := bot.Get(ref.Key())
	r.NoError(err)
	t.Log("content:", string(msg.ContentBytes()))

	mm, ok := msg.(*multimsg.MultiMessage)
	r.True(ok, "wrong type: %T", msg)

	lm, ok := mm.AsLegacy()
	r.True(ok)

	t.Log(string(lm.Raw_))
}
