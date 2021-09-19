// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ssb-ngi-pointer/go-metafeed/metakeys"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestMigrationSentinel(t *testing.T) {
	/* setup start */
	var err error
	r := require.New(t)
	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)
	os.MkdirAll(tRepoPath, 0777)
	/* setup end */

	// sentinel should not exist yet
	exists, err := existsMigrationSentinel(tRepoPath)
	r.False(exists)
	r.NoError(err)

	// create it
	err = persistMigrationSentinel(tRepoPath)
	r.NoError(err)

	// the created sentinel should exist
	exists, err = existsMigrationSentinel(tRepoPath)
	r.True(exists)
	r.NoError(err)
}

func TestMigrate(t *testing.T) {
	/* setup start */
	var err error
	r := require.New(t)
	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)
	bot, err := setupRegularSbot(tRepoPath)
	r.NoError(err)
	mainRef := bot.KeyPair.ID()

	populateTestLog(t, bot)
	shutdownTestBot(t, bot)
	/* setup end */

	// load the classic keypair from disk & read its pubkey
	classicKeypair, err := loadClassicKeypair(tRepoPath)
	r.NoError(err)
	// the main ref containted in the classic secret
	mainRefSecret := classicKeypair.ID()
	r.EqualValues(mainRef, mainRefSecret)

	// move the secret so that we don't start the metafeed sbot from it
	err = moveExistingSecret(tRepoPath)
	r.NoError(err)
	// start a metafeed sbot & get its id (then power it down again; we want the keystore to be available)
	mfId, err := getMetafeedID(tRepoPath)
	r.NoError(err)
	// open the key store and insert the classic key
	err = ingestKeypair(mfId, classicKeypair, tRepoPath)
	r.NoError(err)
	// start the metafeed sbot
	bot, err = setupMetafeedSbot(tRepoPath)
	r.NoError(err)

	aboutMessages, err := getMessages(bot, mainRef, "about")
	r.NoError(err)
	contactMessages, err := getMessages(bot, mainRef, "contact")
	r.NoError(err)

	inform("got messages")

	// create & get the index feeds ontop of the `indexes` subfeed
	aboutIndex, err := bot.MetaFeeds.GetOrCreateIndex(mfId, mainRef, "index", "about")
	r.NoError(err)
	inform("created first index")
	contactIndex, err := bot.MetaFeeds.GetOrCreateIndex(mfId, mainRef, "index", "contact")
	r.NoError(err)
	inform("created two index")

	inform("about index", aboutIndex)
	inform("contact index", contactIndex)

	// post & index the about messages we found in the main feed to the about index
	err = indexMessages(bot, aboutIndex, aboutMessages)
	r.NoError(err)
	// post & index the contact messages we found in the main feed to the contact index
	err = indexMessages(bot, contactIndex, contactMessages)
	r.NoError(err)

	// register indices
	err = bot.MetaFeeds.RegisterIndex(mfId, mainRef, "about")
	r.NoError(err)
	err = bot.MetaFeeds.RegisterIndex(mfId, mainRef, "contact")
	r.NoError(err)

	// post the metafeed/announce message (announcing that "hey i am upgrading to metafeeds!! go get partial w/ me")
	err = postAnnouncement(bot, mainRef)
	r.NoError(err)

	// ACK the main feed from the metafeed, making the meta-ness mutual
	err = registerMainOnMetafeed(bot, mfId, classicKeypair)
	r.NoError(err)

	mfKeypair, ok := bot.KeyPair.(metakeys.KeyPair)
	r.True(ok)
	err = sendSeedInPM(bot, mfKeypair, mainRef)
	r.NoError(err)

	// now dump the migration sentinel to signal the sbot is upgraded
	err = persistMigrationSentinel(tRepoPath)
	r.NoError(err)

	exists, err := existsMigrationSentinel(tRepoPath)
	r.True(exists)
	r.NoError(err)

	shutdownTestBot(t, bot)
}

func shutdownTestBot(t *testing.T, bot *sbot.Sbot) {
	var err error
	r := require.New(t)
	bot.Shutdown()
	err = bot.Close()
	r.NoError(err)
}

// Populate the log with a small subset of messages, some of which will be used to populate specific index feeds later on
func populateTestLog(t *testing.T, bot *sbot.Sbot) {
	var err error
	r := require.New(t)
	mainRef := bot.KeyPair.ID()
	fakeRef1, err := refs.NewFeedRefFromBytes(generateFakePublicKey(1), refs.RefAlgoFeedSSB1)
	r.NoError(err)
	fakeRef2, err := refs.NewFeedRefFromBytes(generateFakePublicKey(2), refs.RefAlgoFeedSSB1)
	r.NoError(err)
	fakeRef3, err := refs.NewFeedRefFromBytes(generateFakePublicKey(3), refs.RefAlgoFeedSSB1)
	r.NoError(err)
	entries := []interface{}{
		refs.NewPost("hello from my main testing feed"),
		refs.NewPost("second post on main"),
		refs.NewAboutName(mainRef, "alpha"),
		refs.NewContactFollow(fakeRef1),
		refs.NewAboutName(mainRef, "beta"),
		refs.NewContactFollow(fakeRef2),
		refs.NewAboutName(mainRef, "omega"),
		refs.NewContactFollow(fakeRef3),
	}

	for _, entry := range entries {
		_, err = bot.PublishLog.Append(entry)
		r.NoError(err)
	}
}

func generateFakePublicKey(num byte) []byte {
	length := 32
	b := make([]byte, length)
	for i := 0; i < len(b); i++ {
		b[i] = num
	}
	return b
}
