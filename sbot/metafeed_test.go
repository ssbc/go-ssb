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
	err = metafeed.VerifySubSignedContent(firstMsg.ContentBytes(), &addMsg, nil)
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
	err = metafeed.VerifySubSignedContent(secondMsg.ContentBytes(), &obituary, nil)
	r.NoError(err)
	r.True(obituary.SubFeed.Equal(subfeedid))

	// try to publish from it
	postRef, err = mainbot.MetaFeeds.Publish(subfeedid, refs.NewPost("still working?!"))
	r.Error(err)

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}

func TestMultiFeedSync(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	// use hmac key
	var hkSecret [32]byte
	_, err := io.ReadFull(rand.Reader, hkSecret[:])
	r.NoError(err)

	ctx, cancel := ShutdownContext(context.Background())
	botgroup, ctx := errgroup.WithContext(ctx)

	logger := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, logger)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	// make the bot
	multiBot, err := New(
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

	r.Equal(multiBot.KeyPair.ID().Algo(), refs.RefAlgoFeedBendyButt)

	// create a two subfeeds
	subfeedClassic, err := multiBot.MetaFeeds.CreateSubFeed("classic", refs.RefAlgoFeedSSB1)
	r.NoError(err)

	subfeedGabby, err := multiBot.MetaFeeds.CreateSubFeed("gabby", refs.RefAlgoFeedGabby)
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
	seqv, err := rxbotsVersionOfmultisMetafeed.Seq().Value()
	r.NoError(err)
	r.EqualValues(1, seqv.(margaret.Seq).Seq(), "should have all of the metafeeds messages")

	r.True(rxbotWantList.Has(subfeedClassic), "rxbot doesn't want classic subfeed")
	r.True(rxbotWantList.Has(subfeedGabby), "rxbot doesn't want gg subfeed")

	// reconnect to iterate and then sync subfeeds
	time.Sleep(3 * time.Second) // wait for hops rebuild

	t.Log("2nd connect: sync the subfeeds")
	err = receiveBot.Network.Connect(ctx, multiBot.Network.GetListenAddr())
	r.NoError(err)

	// wait for all messages to arrive
	wantCount := margaret.BaseSeq(2*n + 2 + 2 - 1) // 2*n test messages on the subfeeds, 2 announcments on the metafeed and two contact messages to befriend the bots
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
