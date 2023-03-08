// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ssbc/go-luigi"
	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/margaret"
	"github.com/stretchr/testify/require"
	"go.mindeco.de/log"
	kitlog "go.mindeco.de/log"
	"golang.org/x/sync/errgroup"

	"github.com/ssbc/go-ssb"
	"github.com/ssbc/go-ssb/internal/leakcheck"
	"github.com/ssbc/go-ssb/internal/storedrefs"
	"github.com/ssbc/go-ssb/internal/testutils"
	"github.com/ssbc/go-ssb/repo"
)

func TestNullFeed(t *testing.T) {
	defer leakcheck.Check(t)
	ctx, cancel := context.WithCancel(context.TODO())
	botgroup, ctx := errgroup.WithContext(ctx)
	logger := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, logger)

	r := require.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(filepath.Join(tRepoPath, "main"))

	// make three new keypairs with nicknames
	n2kp := make(map[string]ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", refs.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", refs.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 2)

	// make the bot
	mainbot, err := New(
		WithInfo(kitlog.With(logger, "bot", "main")),
		WithRepoPath(filepath.Join(tRepoPath, "main")),
		WithHops(2),
		WithHMACSigning(hk),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(mainbot))

	// create some messages under mainbot
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", refs.NewContactFollow(kpBert.ID())},
		{"bert", refs.NewContactFollow(kpArny.ID())},
		{"arny", map[string]interface{}{"type": "test", "hello": 123}},
		{"bert", map[string]interface{}{"type": "test", "world": 456}},
		{"bert", map[string]interface{}{"type": "test", "spew": true, "delete": "me"}},
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

	getUserLog := func(bot *Sbot, name string) margaret.Log {
		kp, has := n2kp[name]
		r.True(has, "%s not in map", name)

		uf, ok := bot.GetMultiLog("userFeeds")
		r.True(ok, "userFeeds mlog not present")

		l, err := uf.Get(storedrefs.Feed(kp.ID()))
		r.NoError(err)

		return l
	}

	checkUserLogSeq := func(bot *Sbot, name string, seq int) {
		l := getUserLog(bot, name)

		checkLogSeq(l, seq)
	}

	// make sure mainbot logged all of our initial test messages
	checkLogSeq(mainbot.ReceiveLog, len(intros)-1) // got all the messages

	// check before drop
	checkUserLogSeq(mainbot, "arny", 1)
	checkUserLogSeq(mainbot, "bert", 2)

	// fsck mainbot before NullFeed so if it fails afterward we know it happened within NullFeed
	err = mainbot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err)

	// delete bert's feed from mainbot
	err = mainbot.NullFeed(kpBert.ID())
	r.NoError(err, "null feed bert failed")

	// fsck mainbot to make sure NullFeed didn't mess something up
	err = mainbot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err)

	// make sure we now have two messages for arny (follow and "hello") and none for bert
	checkUserLogSeq(mainbot, "arny", 1)
	checkUserLogSeq(mainbot, "bert", -1)

	// start an sbot for bert and publish some messages
	bertBot, err := New(
		WithKeyPair(kpBert),
		WithInfo(kitlog.With(logger, "bot", "bert")),
		WithRepoPath(filepath.Join(tRepoPath, "bert")),
		WithHMACSigning(hk),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(bertBot))

	// make main bot follow bert bot so it wants bert's messages
	_, err = mainbot.PublishLog.Publish(refs.NewContactFollow(kpBert.ID()))
	r.NoError(err)

	// make bert bot follow main bot (this does not appear to be strictly necessary)
	_, err = bertBot.PublishLog.Publish(refs.NewContactFollow(mainbot.KeyPair.ID()))
	r.NoError(err)

	// fsck both bots before we try to replicate
	err = mainbot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err)
	err = bertBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err)

	// start the replication processes
	t.Log("starting replication")
	mainbot.Replicate(bertBot.KeyPair.ID())
	bertBot.Replicate(mainbot.KeyPair.ID())

	// publish a bunch more test messages to bertbot so we can make sure they replicate correctly
	const testMsgCount = 1000
	for i := testMsgCount; i > 0; i-- {
		_, err = bertBot.PublishLog.Publish(i)
		r.NoError(err)
	}
	t.Log("done publishing messages")

	// make sure bertbot's log actually reflects all of the messages + the follow message
	checkUserLogSeq(bertBot, "bert", testMsgCount)

	// make sure mainbot still doesn't have any bert messages
	checkUserLogSeq(mainbot, "bert", -1)

	// tell mainbot to connect to bertbot so it will replicate
	t.Log("connecting")
	err = mainbot.Network.Connect(context.TODO(), bertBot.Network.GetListenAddr())
	r.NoError(err)

	// register to be notified any time that mainbot receives a message about bert
	// once we receive all of the messages, continue
	gotMessage := make(chan struct{})
	updateSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, ok := v.(int64)
		if !ok {
			return fmt.Errorf("unexpected type:%T", v)
		}
		s := seq
		if s == testMsgCount-1 { // 0 indexed
			close(gotMessage)
		}
		return err
	})
	betsLog := getUserLog(mainbot, "bert")
	done := betsLog.Changes().Register(updateSink)

	select {
	case <-time.After(25 * time.Second):
		t.Error("sync timeout")

	case <-gotMessage:
		t.Log("re-synced feed")
	}
	done()

	// close things down
	bertBot.Shutdown()
	mainbot.Shutdown()

	cancel()
	r.NoError(bertBot.Close())
	r.NoError(mainbot.Close())

	r.NoError(botgroup.Wait())
}

func TestNullFetched(t *testing.T) {
	if testutils.SkipOnCI(t) {
		// https://github.com/ssbc/go-ssb/pull/167
		return
	}

	defer leakcheck.Check(t)
	r := require.New(t)

	ctx, cancel := context.WithCancel(context.TODO())

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	botgroup, ctx := errgroup.WithContext(ctx)
	mainLog := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, mainLog)

	aliLog := log.With(mainLog, "unit", "ali")
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(aliLog),
		// WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapConn(log.With(aliLog, "who", "a"), conn), nil
		// }),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(ali))

	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "unit", "bob")),
		// WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapConn(bobLog, conn), nil
		// }),
		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(bob))

	ali.Replicate(bob.KeyPair.ID())
	bob.Replicate(ali.KeyPair.ID())

	msgCount := int64(30)
	for i := msgCount; i > 0; i-- {
		c := map[string]interface{}{"test:": i, "type": "test"}
		_, err = bob.PublishLog.Publish(c)
		r.NoError(err)
	}
	firstCtx, firstConnCanel := context.WithCancel(ctx)
	err = bob.Network.Connect(firstCtx, ali.Network.GetListenAddr())
	r.NoError(err)

	alisVersionOfBobsLog, err := ali.Users.Get(storedrefs.Feed(bob.KeyPair.ID()))
	r.NoError(err)

	mainLog.Log("msg", "check we got all the messages")

	gotMessage := make(chan struct{})
	updateSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, ok := v.(int64)
		if !ok {
			return fmt.Errorf("unexpected type:%T", v)
		}
		if int64(seq) == msgCount-1 {
			close(gotMessage)
		}
		return err
	})
	done := alisVersionOfBobsLog.Changes().Register(updateSink)

	select {
	case <-time.After(25 * time.Second):
		t.Error("sync timeout (1)")

	case <-gotMessage:
		t.Log("synced feed")
	}
	done()

	ali.Network.GetConnTracker().CloseAll()
	bob.Network.GetConnTracker().CloseAll()
	firstConnCanel()

	err = ali.NullFeed(bob.KeyPair.ID())
	r.NoError(err)

	mainLog.Log("msg", "get a fresh view (should be empty now)")

	r.EqualValues(margaret.SeqSublogDeleted, alisVersionOfBobsLog.Seq(), "TODO: error log value deleted?!")

	alisVersionOfBobsLog, err = ali.Users.Get(storedrefs.Feed(bob.KeyPair.ID()))
	r.NoError(err)

	r.EqualValues(margaret.SeqEmpty, alisVersionOfBobsLog.Seq())

	mainLog.Log("msg", "get a fresh view (should be empty now)")

	ali.Replicate(bob.KeyPair.ID())
	bob.Replicate(ali.KeyPair.ID())

	mainLog.Log("msg", "sync should give us the messages again")
	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)

	// start := time.Now()
	gotMessage = make(chan struct{})
	updateSink = luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, ok := v.(int64)
		if !ok {
			return fmt.Errorf("unexpected type:%T", v)
		}
		if int64(seq) == msgCount-1 {
			close(gotMessage)
		}
		return err
	})

	done = alisVersionOfBobsLog.Changes().Register(updateSink)
	select {
	case <-time.After(25 * time.Second):
		t.Error("sync timeout (2)")

	case <-gotMessage:
		t.Log("re-synced feed")
	}
	done()

	ali.Shutdown()
	bob.Shutdown()
	cancel()

	r.NoError(ali.Close())
	r.NoError(bob.Close())

	r.NoError(botgroup.Wait())
}
