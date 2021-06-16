package sbot

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"
)

func TestMigrateFromMultiFeed(t *testing.T) {
	// create a repo with a ssb v1 keypair

	// create a bunch of messages with types contact and post

	// restart with metafeed mode

	// assert metafeed keypair is created

	// assert old/main-feed and metafeed are linked

	// FUTURE: assert creation of index-feeds for post types
}

func TestMultiFeedManagment(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	// hmac key for this test
	hk := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, hk)
	r.NoError(err)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	mainbot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		WithHMACSigning(hk),
		DisableNetworkNode(),
		WithMetaFeedMode(true),
	)
	r.NoError(err)

	subfeedid, err := mainbot.MetaFeeds.CreateSubFeed(t.Name(), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	lst, err := mainbot.MetaFeeds.ListSubFeeds()
	r.NoError(err)
	r.Len(lst, 1)
	r.True(lst[0].Feed.Equal(subfeedid))
	r.Equal(lst[0].Purpose, t.Name())

	postRef, err := mainbot.MetaFeeds.Publish(subfeedid, refs.NewPost("hello from my testing subfeed"))
	r.NoError(err)

	err = mainbot.MetaFeeds.TombstoneSubFeed(subfeedid)
	r.NoError(err)

	lst, err = mainbot.MetaFeeds.ListSubFeeds()
	r.NoError(err)
	r.Len(lst, 0)

	postRef, err = mainbot.MetaFeeds.Publish(subfeedid, refs.NewPost("still working?!"))
	r.Error(err)
	r.Nil(postRef)

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}
