package sbot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"
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

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	mainbot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
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

	var addMsg metamngmt.Add
	err = metafeed.VerifySubSignedContent(firstMsg.ContentBytes(), &addMsg)
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

func TestMultiFeedManualSync(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	// TODO: enable hmac key
	// hk := make([]byte, 32)
	// _, err := io.ReadFull(rand.Reader, hk)
	// r.NoError(err)

	ctx, cancel := ShutdownContext(context.Background())
	botgroup, ctx := errgroup.WithContext(ctx)

	logger := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, logger)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	// make the bot
	multiBot, err := New(
		WithInfo(logger),
		WithRepoPath(filepath.Join(tRepoPath, "mfbot")),
		WithListenAddr(":0"),
		// WithHMACSigning(hk),
		WithMetaFeedMode(true),
		DisableEBT(true), // TODO: have different formats in ebt
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(multiBot))

	r.Equal(multiBot.KeyPair.ID().Algo(), refs.RefAlgoFeedBendyButt)

	// create a two subfeeds
	subfeedClassic, err := multiBot.MetaFeeds.CreateSubFeed("classic", refs.RefAlgoFeedSSB1)
	r.NoError(err)

	subfeedGabby, err := multiBot.MetaFeeds.CreateSubFeed("gabby", refs.RefAlgoFeedGabby)
	r.NoError(err)

	// create some spam
	n := 100
	for i := n; i > 0; i-- {
		testSpam := fmt.Sprintf("hello from my testing subfeed %d", i)

		_, err = multiBot.MetaFeeds.Publish(subfeedClassic, refs.NewPost(testSpam+" (classic)"))
		r.NoError(err)
		_, err = multiBot.MetaFeeds.Publish(subfeedGabby, refs.NewPost(testSpam+" (gabby)"))
		r.NoError(err)
	}

	// now start the receiving bot
	receiveBot, err := New(
		WithInfo(logger),
		WithRepoPath(filepath.Join(tRepoPath, "rxbot")),
		WithListenAddr(":0"),
		// WithHMACSigning(hk),
		DisableEBT(true), // TODO: have different formats in ebt
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(receiveBot))

	// replicate between the two

	multiBot.Replicate(receiveBot.KeyPair.ID())

	receiveBot.Replicate(multiBot.KeyPair.ID())
	receiveBot.Replicate(subfeedClassic)
	receiveBot.Replicate(subfeedGabby)

	// sync
	err = receiveBot.Network.Connect(ctx, multiBot.Network.GetListenAddr())
	r.NoError(err)

	// wait for all messages to arrive
	wantCount := margaret.BaseSeq(2*n + 2 - 1)
	src, err := receiveBot.ReceiveLog.Query(margaret.Gte(wantCount), margaret.Live(true))
	r.NoError(err)
	ctx, tsCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tsCancel()
	v, err := src.Next(ctx)
	r.NoError(err)
	t.Log(v.(refs.Message).Key().Ref())

	// shutdown
	cancel()
	multiBot.Shutdown()
	r.NoError(multiBot.Close())
	receiveBot.Shutdown()
	r.NoError(receiveBot.Close())

	r.NoError(botgroup.Wait())
}
