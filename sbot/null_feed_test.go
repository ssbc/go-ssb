package sbot

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/repo"
)

func TestNullFeed(t *testing.T) {
	// // defer leakcheck.Check(t)
	// ctx := context.Background()
	ctx, cancel := context.WithCancel(context.TODO())
	botgroup, ctx := errgroup.WithContext(ctx)

	r := require.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(filepath.Join(tRepoPath, "main"))

	// make three new keypairs with nicknames
	n2kp := make(map[string]*ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", ssb.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 2)

	// make the bot
	logger := testutils.NewRelativeTimeLogger(nil)
	mainbot, err := New(
		WithInfo(kitlog.With(logger, "bot", "main")),
		WithRepoPath(filepath.Join(tRepoPath, "main")),
		WithHops(2),
		WithHMACSigning(hk),
		LateOption(MountSimpleIndex("get", indexes.OpenGet)),
		WithListenAddr(":0"),
	)
	r.NoError(err)

	botgroup.Go(func() error {
		err := mainbot.Network.Serve(ctx)
		if err != nil {
			level.Warn(logger).Log("event", "bob serve exited", "err", err)
		}
		return err
	})

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", ssb.NewContactFollow(kpBert.Id)},
		{"bert", ssb.NewContactFollow(kpArny.Id)},
		{"arny", map[string]interface{}{"hello": 123}},
		{"bert", map[string]interface{}{"world": 456}},
		{"bert", map[string]interface{}{"spew": true, "delete": "me"}},
	}

	for idx, intro := range intros {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
		msg, err := mainbot.Get(*ref)
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

	checkUserLogSeq := func(bot *Sbot, name string, seq int) {
		kp, has := n2kp[name]
		r.True(has, "%s not in map", name)

		uf, ok := bot.GetMultiLog("userFeeds")
		r.True(ok, "userFeeds mlog not present")

		l, err := uf.Get(kp.Id.StoredAddr())
		r.NoError(err)

		checkLogSeq(l, seq)
	}

	checkLogSeq(mainbot.RootLog, len(intros)-1) // got all the messages

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
	botgroup.Go(func() error {
		err := bertBot.Network.Serve(ctx)
		if err != nil {
			level.Warn(logger).Log("event", "bob serve exited", "err", err)
		}
		return err
	})

	// make main want it
	_, err = mainbot.PublishLog.Publish(ssb.NewContactFollow(kpBert.Id))
	r.NoError(err)

	_, err = bertBot.PublishLog.Publish(ssb.NewContactFollow(mainbot.KeyPair.Id))
	r.NoError(err)

	for i := 1000; i > 0; i-- {
		_, err = bertBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	err = mainbot.Network.Connect(context.TODO(), bertBot.Network.GetListenAddr())
	r.NoError(err)

	time.Sleep(8 * time.Second)
	checkUserLogSeq(mainbot, "bert", 1000)

	bertBot.Shutdown()
	mainbot.Shutdown()

	r.NoError(bertBot.Close())
	r.NoError(mainbot.Close())

	cancel()
	r.NoError(botgroup.Wait())
}

func TestNullFetched(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ctx, cancel := context.WithCancel(context.TODO())

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	botgroup, ctx := errgroup.WithContext(ctx)

	mainLog := testutils.NewRelativeTimeLogger(nil)

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
		// LateOption(MountPlugin(&bytype.Plugin{}, plugins2.AuthMaster)),
	)
	r.NoError(err)

	botgroup.Go(func() error {
		err := ali.Network.Serve(ctx)
		if err != nil {
			level.Warn(mainLog).Log("event", "ali serve exited", "err", err)
		}
		return err
	})

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
		// LateOption(MountPlugin(&bytype.Plugin{}, plugins2.AuthMaster)),
	)
	r.NoError(err)

	botgroup.Go(func() error {
		err := bob.Network.Serve(ctx)
		if err != nil {
			level.Warn(mainLog).Log("event", "bob serve exited", "err", err)
		}
		return err
	})

	seq, err := ali.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   bob.KeyPair.Id,
	})
	r.NoError(err)
	r.Equal(margaret.BaseSeq(0), seq)

	seq, err = bob.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   ali.KeyPair.Id,
	})
	r.NoError(err)

	for i := 1000; i > 0; i-- {
		_, err = bob.PublishLog.Publish(i)
		r.NoError(err)
	}

	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(3 * time.Second)

	aliUF, ok := ali.GetMultiLog("userFeeds")
	r.True(ok)

	alisVersionOfBobsLog, err := aliUF.Get(bob.KeyPair.Id.StoredAddr())
	r.NoError(err)

	mainLog.Log("msg", "check we got all the messages")
	bobsSeqV, err := alisVersionOfBobsLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(1000, bobsSeqV.(margaret.Seq).Seq())

	err = ali.NullFeed(bob.KeyPair.Id)
	r.NoError(err)

	mainLog.Log("msg", "get a fresh view (shoild be empty now)")
	alisVersionOfBobsLog, err = aliUF.Get(bob.KeyPair.Id.StoredAddr())
	r.NoError(err)

	bobsSeqV, err = alisVersionOfBobsLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(margaret.SeqEmpty, bobsSeqV.(margaret.Seq).Seq())

	mainLog.Log("msg", "sync should give us the messages again")
	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(3 * time.Second)

	bobsSeqV, err = alisVersionOfBobsLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(1000, bobsSeqV.(margaret.Seq).Seq())

	ali.Shutdown()
	bob.Shutdown()

	r.NoError(ali.Close())
	r.NoError(bob.Close())

	cancel()
	r.NoError(botgroup.Wait())
}
