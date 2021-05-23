package sbot

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.mindeco.de/log"
	kitlog "go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/repo"
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
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(mainbot))

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", refs.NewContactFollow(kpBert.Id)},
		{"bert", refs.NewContactFollow(kpArny.Id)},
		{"arny", map[string]interface{}{"type": "test", "hello": 123}},
		{"bert", map[string]interface{}{"type": "test", "world": 456}},
		{"bert", map[string]interface{}{"type": "test", "spew": true, "delete": "me"}},
	}

	for idx, intro := range intros {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
		msg, err := mainbot.Get(ref)
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

	getUserLog := func(bot *Sbot, name string) margaret.Log {
		kp, has := n2kp[name]
		r.True(has, "%s not in map", name)

		uf, ok := bot.GetMultiLog("userFeeds")
		r.True(ok, "userFeeds mlog not present")

		l, err := uf.Get(storedrefs.Feed(kp.Id))
		r.NoError(err)

		return l
	}

	checkUserLogSeq := func(bot *Sbot, name string, seq int) {
		l := getUserLog(bot, name)

		checkLogSeq(l, seq)
	}

	checkLogSeq(mainbot.ReceiveLog, len(intros)-1) // got all the messages

	// check before drop
	checkUserLogSeq(mainbot, "arny", 1)
	checkUserLogSeq(mainbot, "bert", 2)

	err = mainbot.NullFeed(kpBert.Id)
	r.NoError(err, "null feed bert failed")

	checkUserLogSeq(mainbot, "arny", 1)
	checkUserLogSeq(mainbot, "bert", -1)

	// start bert and publish some messages
	bertBot, err := New(
		WithKeyPair(kpBert),
		WithInfo(kitlog.With(logger, "bot", "bert")),
		WithRepoPath(filepath.Join(tRepoPath, "bert")),
		WithHMACSigning(hk),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(bertBot))

	// make main want it
	_, err = mainbot.PublishLog.Publish(refs.NewContactFollow(kpBert.Id))
	r.NoError(err)

	_, err = bertBot.PublishLog.Publish(refs.NewContactFollow(mainbot.KeyPair.Id))
	r.NoError(err)

	mainbot.Replicate(bertBot.KeyPair.Id)
	bertBot.Replicate(mainbot.KeyPair.Id)

	const testMsgCount = 1000
	for i := testMsgCount; i > 0; i-- {
		_, err = bertBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	err = mainbot.Network.Connect(context.TODO(), bertBot.Network.GetListenAddr())
	r.NoError(err)

	gotMessage := make(chan struct{})
	updateSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, ok := v.(margaret.Seq)
		if !ok {
			return fmt.Errorf("unexpected type:%T", v)
		}
		s := seq.Seq()
		if s == testMsgCount-1 { // 0 indexed
			close(gotMessage)
		}
		return err
	})
	betsLog := getUserLog(mainbot, "bert")
	done := betsLog.Seq().Register(updateSink)

	select {
	case <-time.After(25 * time.Second):
		t.Error("sync timeout")

	case <-gotMessage:
		t.Log("re-synced feed")
	}
	done()

	bertBot.Shutdown()
	mainbot.Shutdown()

	cancel()
	r.NoError(bertBot.Close())
	r.NoError(mainbot.Close())

	r.NoError(botgroup.Wait())
}

func TestNullFetched(t *testing.T) {
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

	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "unit", "ali")),
		// WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapConn(log.With(aliLog, "who", "a"), conn), nil
		// }),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
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
	)
	r.NoError(err)

	botgroup.Go(bs.Serve(bob))

	ali.Replicate(bob.KeyPair.Id)
	bob.Replicate(ali.KeyPair.Id)

	for i := 1000; i > 0; i-- {
		c := map[string]interface{}{"test:": i, "type": "test"}
		_, err = bob.PublishLog.Publish(c)
		r.NoError(err)
	}

	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)

	aliUF, ok := ali.GetMultiLog("userFeeds")
	r.True(ok)

	alisVersionOfBobsLog, err := aliUF.Get(storedrefs.Feed(bob.KeyPair.Id))
	r.NoError(err)

	mainLog.Log("msg", "check we got all the messages")

	gotMessage := make(chan struct{})
	updateSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, ok := v.(margaret.Seq)
		if !ok {
			return fmt.Errorf("unexpected type:%T", v)
		}
		s := seq.Seq()
		if s == 999 {
			close(gotMessage)
		}
		return err
	})

	done := alisVersionOfBobsLog.Seq().Register(updateSink)

	select {
	case <-time.After(25 * time.Second):
		t.Error("sync timeout (1)")

	case <-gotMessage:
		t.Log("synced feed")
	}
	done()

	err = ali.NullFeed(bob.KeyPair.Id)
	r.NoError(err)

	mainLog.Log("msg", "get a fresh view (shoild be empty now)")
	alisVersionOfBobsLog, err = aliUF.Get(storedrefs.Feed(bob.KeyPair.Id))
	r.NoError(err)

	mainLog.Log("msg", "sync should give us the messages again")
	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)

	// start := time.Now()
	gotMessage = make(chan struct{})
	updateSink = luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, ok := v.(margaret.Seq)
		if !ok {
			return fmt.Errorf("unexpected type:%T", v)
		}
		s := seq.Seq()
		if s == 999 {
			close(gotMessage)
		}
		return err
	})

	done = alisVersionOfBobsLog.Seq().Register(updateSink)
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
