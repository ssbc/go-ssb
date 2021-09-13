// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"
)

func TestMigrateFromMetaFeed(t *testing.T) {
	// create a repo with a ssb v1 keypair

	// create a bunch of messages with types contact and post

	// restart with metafeed mode

	// assert metafeed keypair is created

	// assert old/main-feed and metafeed are linked

	// FUTURE: assert creation of index-feeds for post types
}

func TestMetafeedManagment(t *testing.T) {
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

	storedMetafeed, err := mainbot.Users.Get(storedrefs.Feed(mainbot.KeyPair.ID()))
	r.NoError(err)
	var checkSeq = func(want int) refs.Message {
		r.EqualValues(want, storedMetafeed.Seq())

		if want == -1 {
			return nil
		}

		rxSeq, err := storedMetafeed.Get(int64(want))
		r.NoError(err)

		mv, err := mainbot.ReceiveLog.Get(rxSeq.(int64))
		r.NoError(err)

		return mv.(refs.Message)
	}

	checkSeq(int(margaret.SeqEmpty))

	// create a new subfeed
	subfeedid, err := mainbot.MetaFeeds.CreateSubFeed(mainbot.KeyPair.ID(), t.Name(), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// list it
	lst, err := mainbot.MetaFeeds.ListSubFeeds(mainbot.KeyPair.ID())
	r.NoError(err)
	r.Len(lst, 1)
	r.True(lst[0].Feed.Equal(subfeedid))
	// TODO (2021-09-16): re-enable test after implementing routine for getting msg @ seqno
	// r.Equal(lst[0].Purpose, t.Name())

	// check we published the new sub-feed on the metafeed
	firstMsg := checkSeq(int(0))

	var addMsg metamngmt.AddDerived
	err = metafeed.VerifySubSignedContent(firstMsg.ContentBytes(), &addMsg)
	r.NoError(err)
	r.True(addMsg.SubFeed.Equal(subfeedid))

	// publish from it
	postMsg, err := mainbot.MetaFeeds.Publish(subfeedid, refs.NewPost("hello from my testing subfeed"))
	r.NoError(err)
	t.Log(postMsg.Key().String())

	// check it has the msg
	subfeedLog, err := mainbot.Users.Get(storedrefs.Feed(subfeedid))
	r.NoError(err)

	r.EqualValues(0, subfeedLog.Seq())

	// drop it
	err = mainbot.MetaFeeds.TombstoneSubFeed(mainbot.KeyPair.ID(), subfeedid)
	r.NoError(err)

	// shouldnt be listed as active
	lst, err = mainbot.MetaFeeds.ListSubFeeds(mainbot.KeyPair.ID())
	r.NoError(err)
	r.Len(lst, 0)

	// check we published the tombstone update on the metafeed
	secondMsg := checkSeq(int(1))

	var obituary metamngmt.Tombstone
	err = metafeed.VerifySubSignedContent(secondMsg.ContentBytes(), &obituary)
	r.NoError(err)
	r.True(obituary.SubFeed.Equal(subfeedid))

	// try to publish from it
	_, err = mainbot.MetaFeeds.Publish(subfeedid, refs.NewPost("still working?!"))
	r.Error(err)

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}

func TestMetafeedSync(t *testing.T) {
	r := require.New(t)

	// use hmac key
	var hkSecret [32]byte
	_, err := io.ReadFull(rand.Reader, hkSecret[:])
	r.NoError(err)

	ctx, botShutdown := ShutdownContext(context.Background())
	botgroup, ctx := errgroup.WithContext(ctx)

	logger := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, logger)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	// make the bot
	multiBot, err := New(
		WithContext(ctx),
		WithInfo(log.With(logger, "bot", "creater")),
		WithRepoPath(filepath.Join(tRepoPath, "mfbot")),
		WithListenAddr(":0"),
		WithHMACSigning(hkSecret[:]),
		WithWebsocketAddress("localhost:12345"),
		WithMetaFeedMode(true),
		DisableEBT(true), // TODO: have different formats in ebt
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(multiBot))

	multibotFeed := multiBot.KeyPair.ID()
	r.Equal(multibotFeed.Algo(), refs.RefAlgoFeedBendyButt)

	// create a two subfeeds
	subfeedClassic, err := multiBot.MetaFeeds.CreateSubFeed(multibotFeed, "classic", refs.RefAlgoFeedSSB1)
	r.NoError(err)

	subfeedGabby, err := multiBot.MetaFeeds.CreateSubFeed(multibotFeed, "gabby", refs.RefAlgoFeedGabby)
	r.NoError(err)

	// create some spam
	n := 5
	for i := n; i > 0; i-- {
		testSpam := fmt.Sprintf("hello from my testing subfeed %d", i)

		_, err = multiBot.MetaFeeds.Publish(subfeedClassic, refs.NewPost(testSpam+" (classic)"))
		r.NoError(err)
		_, err = multiBot.MetaFeeds.Publish(subfeedGabby, refs.NewPost(testSpam+" (gabby)"))
		r.NoError(err)
	}

	rxBotKeypair, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// now start the receiving bot
	receiveBot, err := New(
		WithContext(ctx),
		WithInfo(log.With(logger, "bot", "receiver")),
		WithRepoPath(filepath.Join(tRepoPath, "rxbot")),
		WithListenAddr(":0"),
		WithHMACSigning(hkSecret[:]),

		// use metafeeds but dont have one
		WithKeyPair(rxBotKeypair),
		WithMetaFeedMode(true),

		DisableEBT(true), // TODO: have different formats in ebt
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(receiveBot))

	// replicate between the two
	_, err = multiBot.MetaFeeds.Publish(subfeedClassic, refs.NewContactFollow(receiveBot.KeyPair.ID()))
	r.NoError(err)
	_, err = receiveBot.PublishLog.Publish(refs.NewContactFollow(multiBot.KeyPair.ID()))
	r.NoError(err)

	time.Sleep(4 * time.Second) // graph update delay

	multibotWantList := multiBot.Replicator.Lister().ReplicationList()
	r.True(multibotWantList.Has(receiveBot.KeyPair.ID()), "multi doesnt want to peer with rxbot. Count:%d", multibotWantList.Count())

	t.Log("first sync: get the metafeed")
	firstConnectCtx, cancel := context.WithCancel(ctx)
	err = receiveBot.Network.Connect(firstConnectCtx, multiBot.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(5 * time.Second)
	cancel()
	receiveBot.Network.GetConnTracker().CloseAll()

	// check we got all the messages
	rxbotWantList := receiveBot.Replicator.Lister().ReplicationList()

	rxbotsVersionOfmultisMetafeed, err := receiveBot.Users.Get(storedrefs.Feed(multiBot.KeyPair.ID()))
	r.NoError(err)

	// check rxbot got the metafeed
	r.True(rxbotWantList.Has(multiBot.KeyPair.ID()), "rxbot doesn't want mutlibots metafeed")

	r.EqualValues(1, rxbotsVersionOfmultisMetafeed.Seq(), "should have all of the metafeeds messages")

	r.True(rxbotWantList.Has(subfeedClassic), "rxbot doesn't want classic subfeed")
	r.True(rxbotWantList.Has(subfeedGabby), "rxbot doesn't want gg subfeed")

	// reconnect to iterate and then sync subfeeds
	time.Sleep(3 * time.Second) // wait for hops rebuild

	t.Log("2nd connect: sync the subfeeds")
	err = receiveBot.Network.Connect(ctx, multiBot.Network.GetListenAddr())
	r.NoError(err)

	// wait for all messages to arrive
	wantCount := int64(2*n + 2 + 2 - 1) // 2*n test messages on the subfeeds, 2 announcments on the metafeed and two contact messages to befriend the bots
	src, err := receiveBot.ReceiveLog.Query(margaret.Gte(wantCount), margaret.Live(true))
	r.NoError(err)
	ctx, tsCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tsCancel()
	v, err := src.Next(ctx)
	r.NoError(err)
	t.Log(v.(refs.Message).Key().String())

	// shutdown
	botShutdown()
	multiBot.Shutdown()
	r.NoError(multiBot.Close())
	receiveBot.Shutdown()
	r.NoError(receiveBot.Close())

	r.NoError(botgroup.Wait())
}

func TestMetafeedInsideMetafeed(t *testing.T) {
	// <boilerplate>
	r := require.New(t)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	bot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		DisableNetworkNode(),
		WithMetaFeedMode(true),
	)
	r.NoError(err)
	// </boilerplate>
	mfId := bot.KeyPair.ID()

	// create an `indexes` subfeed, which is a metafeed, on the "root" metafeed
	indexesFeed, err := bot.MetaFeeds.CreateSubFeed(mfId, "indexes", refs.RefAlgoFeedBendyButt)
	r.NoError(err)

	// now create two index feeds on the just created subfeed for indexes
	idx1, err := bot.MetaFeeds.CreateSubFeed(indexesFeed, "index-foo", refs.RefAlgoFeedGabby)
	r.NoError(err)

	idx2, err := bot.MetaFeeds.CreateSubFeed(indexesFeed, "index-bar", refs.RefAlgoFeedGabby)
	r.NoError(err)

	/* first up: let's check that we can list them */
	// check that the root mf only has one feed (i.e. idxSubFeed)
	lst, err := bot.MetaFeeds.ListSubFeeds(mfId)
	r.NoError(err)
	r.Len(lst, 1, "the root mf has more than one subfeed")
	r.True(lst[0].Feed.Equal(indexesFeed))

	// then, verify that the subfeed has the two new index feeds
	lst, err = bot.MetaFeeds.ListSubFeeds(indexesFeed)
	r.NoError(err)
	r.Len(lst, 2, "more than two indexes on the subfeed")

	// write contains util to make sure that the two indexes exist (not guaranteed to be ordered)
	has := func(feedlist []ssb.SubfeedListEntry, target refs.FeedRef) bool {
		var found bool
		for _, entry := range feedlist {
			if entry.Feed.Equal(idx1) {
				found = true
				break
			}
		}
		return found
	}
	r.True(has(lst, idx1), "found idx1")
	r.True(has(lst, idx2), "found idx2")

	// check we can publish as idx1 and idx2
	_, err = bot.MetaFeeds.Publish(idx1, refs.NewPost("foo!"))
	r.NoError(err)
	_, err = bot.MetaFeeds.Publish(idx2, refs.NewPost("bar!"))
	r.NoError(err)

	// try to tombstone on the wrong mount
	err = bot.MetaFeeds.TombstoneSubFeed(mfId, idx1)
	r.Error(err)

	// listings stay the same
	lst, err = bot.MetaFeeds.ListSubFeeds(mfId)
	r.NoError(err)
	r.Len(lst, 1, "the root mf has more than one subfeed after faulty tombstone")

	lst, err = bot.MetaFeeds.ListSubFeeds(indexesFeed)
	r.NoError(err)
	r.Len(lst, 2, "more than two indexes on the subfeed after faulty tombstone")

	// now remove index from the right mount
	err = bot.MetaFeeds.TombstoneSubFeed(indexesFeed, idx1)
	r.NoError(err)

	lst, err = bot.MetaFeeds.ListSubFeeds(indexesFeed)
	r.NoError(err)
	r.Len(lst, 1, "more than 1 subfeed on the index")

	// <teardown>
	bot.Shutdown()
	r.NoError(bot.Close())
	// </teardown>
}

func TestMetafeedIndexes(t *testing.T) {
	// <boilerplate>
	r := require.New(t)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	bot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		DisableNetworkNode(),
		WithMetaFeedMode(true),
	)
	r.NoError(err)
	// </boilerplate>

	/* test index feed creation */
	// the test plan:
	// * have a main feed (ssb1)
	// * create indexes
	// * publish messages on the main
	// * check that the published msgs corresponding index messages exist

	// util funcs
	var checkSeq = func(feed margaret.Log, want int) refs.Message {
		if want == -1 {
			return nil
		}

		want = want - 1
		r.EqualValues(want, feed.Seq())

		rxSeq, err := feed.Get(int64(want))
		r.NoError(err)

		// mv == message value (because of the empty interface assert)
		mv, err := bot.ReceiveLog.Get(rxSeq.(int64))
		r.NoError(err)

		return mv.(refs.Message)
	}

	var getFeed = func (feedId refs.FeedRef) (margaret.Log) {
		feed, err := bot.Users.Get(storedrefs.Feed(feedId))
		r.NoError(err)
		return feed
	}

	mfId := bot.KeyPair.ID()
	// create a main feed (holds actual messages, regular old ssb feed thinger) on the root metafeed
	mainFeedRef, err := bot.MetaFeeds.CreateSubFeed(mfId, "main", refs.RefAlgoFeedSSB1)
	r.NoError(err, "main feed create failed")

	// register an index for about messages
	err = bot.MetaFeeds.RegisterIndex(mfId, mainFeedRef, "about")
	r.NoError(err)

	// register an index for contact (follow) messages
	err = bot.MetaFeeds.RegisterIndex(mfId, mainFeedRef, "contact")
	r.NoError(err)

	// get the actual index feeds so we can assert on them
	aboutIndexId, err := bot.MetaFeeds.GetOrCreateIndex(mfId, mainFeedRef, "index", "about")
	r.NoError(err)
	aboutIndex := getFeed(aboutIndexId)
	checkSeq(aboutIndex, int(margaret.SeqEmpty))

	contactIndexId, err := bot.MetaFeeds.GetOrCreateIndex(mfId, mainFeedRef, "index", "contact")
	r.NoError(err)
	contactIndex := getFeed(contactIndexId)
	checkSeq(contactIndex, int(margaret.SeqEmpty))

	/* publish an about to the main feed */
	_, err = bot.MetaFeeds.Publish(mainFeedRef, refs.NewAboutName(mainFeedRef, "goophy"))
	r.NoError(err)
	// contact index should still be empty
	checkSeq(contactIndex, int(margaret.SeqEmpty))
	// about index should have one message
	checkSeq(aboutIndex, 1)

	/* publish a contact message to the main feed */
	tRepo := repo.New(tRepoPath)
	rando, err := repo.NewKeyPair(tRepo, "rando", refs.RefAlgoFeedSSB1)
	_, err = bot.MetaFeeds.Publish(mainFeedRef, refs.NewContactFollow(rando.ID()))
	r.NoError(err)
	// contact index should now have a message
	checkSeq(contactIndex, 1)
	// about index should still have one message
	checkSeq(aboutIndex, 1)

	/* post another about and verify that the index is being updated */
	// publish another about to the main feed
	_, err = bot.MetaFeeds.Publish(mainFeedRef, refs.NewAboutName(mainFeedRef, "goofier"))
	r.NoError(err)
	// about index should now have two messages
	checkSeq(aboutIndex, 2)
	// contact index still only have one message
	checkSeq(contactIndex, 1)

	/* tombstone the about index, and verify that the index is not being updated */
	err = bot.MetaFeeds.TombstoneSubFeed(mfId, aboutIndexId)
	r.NoError(err)
	// publish another about to the main feed
	_, err = bot.MetaFeeds.Publish(mainFeedRef, refs.NewAboutName(mainFeedRef, "name that will not be named"))
	r.NoError(err)
	// about index should still have have two messages?
	checkSeq(aboutIndex, 2)

	// <teardown>
	bot.Shutdown()
	bot.Close()
}

// to test the restarting of the sbot, do the teardown and then the setup again and issue some asserts basically
func TestMetafeedIndexesReboot(t *testing.T) {
	// <boilerplate>
	r := require.New(t)

	// use same repo as previous test
	tRepoPath := "testrun/TestMetafeedIndexes"

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	bot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		DisableNetworkNode(),
		WithMetaFeedMode(true),
	)
	r.NoError(err)
	// </boilerplate>
	fmt.Println("restarted bot id", bot.KeyPair.ID())
}
