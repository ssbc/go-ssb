package sbot

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/storedrefs"
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

	r.Equal(mainbot.KeyPair.ID().Algo(), refs.RefAlgoFeedBendyButt)

	// did b0 get feed of bN-1?
	storedMetafeed, err := mainbot.Users.Get(storedrefs.Feed(mainbot.KeyPair.ID()))
	r.NoError(err)
	var checkSeq = func(want int) refs.Message {
		seqv, err := storedMetafeed.Seq().Value()
		r.NoError(err)
		r.EqualValues(want, seqv)

		if want == -1 {
			return nil
		}

		rxSeq, err := storedMetafeed.Get(margaret.BaseSeq(want))
		r.NoError(err)

		mv, err := mainbot.ReceiveLog.Get(rxSeq.(margaret.BaseSeq))
		r.NoError(err)

		return mv.(refs.Message)
	}

	checkSeq(int(margaret.SeqEmpty))

	// create a new subfeed
	subfeedid, err := mainbot.MetaFeeds.CreateSubFeed(t.Name(), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// list it
	lst, err := mainbot.MetaFeeds.ListSubFeeds()
	r.NoError(err)
	r.Len(lst, 1)
	r.True(lst[0].Feed.Equal(subfeedid))
	r.Equal(lst[0].Purpose, t.Name())

	// check we published the new sub-feed on the metafeed
	firstMsg := checkSeq(int(0))

	firstContent := firstMsg.ContentBytes()
	// hexer := hex.Dumper(os.Stderr)
	// hexer.Write(firstContent)
	// os.Stderr.WriteString("\n")
	// os.Stderr.WriteString(hex.EncodeToString(firstContent))

	var addMsg metamngmt.Add
	err = metafeed.VerifySubSignedContent(firstContent, &addMsg)
	r.NoError(err)
	r.True(addMsg.SubFeed.Equal(subfeedid))

	// publish from it
	postRef, err := mainbot.MetaFeeds.Publish(subfeedid, refs.NewPost("hello from my testing subfeed"))
	r.NoError(err)
	t.Log(postRef.Ref())

	// check it has the msg
	subfeedLog, err := mainbot.Users.Get(storedrefs.Feed(subfeedid))
	r.NoError(err)

	subfeedLen, err := subfeedLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(0, subfeedLen.(margaret.Seq).Seq())

	// drop it
	err = mainbot.MetaFeeds.TombstoneSubFeed(subfeedid)
	r.NoError(err)

	// shouldnt be listed as active
	lst, err = mainbot.MetaFeeds.ListSubFeeds()
	r.NoError(err)
	r.Len(lst, 0)

	// check we published the tombstone update on the metafeed
	secondMsg := checkSeq(int(1))
	t.Log(hex.Dump(secondMsg.ContentBytes()))

	var obituary metamngmt.Tombstone
	err = metafeed.VerifySubSignedContent(secondMsg.ContentBytes(), &obituary)
	r.NoError(err)
	r.True(obituary.SubFeed.Equal(subfeedid))

	// try to publish from it
	postRef, err = mainbot.MetaFeeds.Publish(subfeedid, refs.NewPost("still working?!"))
	r.Error(err)

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}
