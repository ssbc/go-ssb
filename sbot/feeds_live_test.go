// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/VividCortex/gohistogram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/network"
	refs "go.mindeco.de/ssb-refs"
)

var (
	// how many messages will be published (if -short isnt used)
	testMessageCount = 128

	// a running tally of how many times makeNamedTestBot() was called
	// is also used to alternate between feed formats per bot
	botCnt byte
) //

func init() {

	// shifts the keypair order around each time
	// so the order is shifted sometimes (botA is GG format instead of legacy sometimes)
	botCnt = byte(time.Now().Unix() % 255)
}

func makeNamedTestBot(t testing.TB, name string, opts []Option) *Sbot {
	r := require.New(t)
	testPath := filepath.Join("testrun", t.Name(), "bot-"+name)

	if testing.Short() {
		testMessageCount = 15
	}

	// bob is the one with the other feed format

	// make keys deterministic each run
	seed := bytes.Repeat([]byte{botCnt}, 32)
	botCnt++

	algo := refs.RefAlgoFeedSSB1
	/* TODO: fix gabby support
	if botCnt%2 == 0 {
		algo = refs.RefAlgoFeedGabby
	}
	*/

	botsKey, err := ssb.NewKeyPair(bytes.NewReader(seed), algo)
	r.NoError(err)

	mainLog := log.NewNopLogger()
	if testing.Verbose() {
		mainLog = testutils.NewRelativeTimeLogger(nil)
	}
	botOptions := append(opts,
		WithKeyPair(botsKey),
		WithInfo(log.With(mainLog, "bot", name)),
		WithRepoPath(testPath),
		WithListenAddr(":0"),
		WithNetworkConnTracker(network.NewLastWinsTracker()),
	)

	theBot, err := New(botOptions...)
	r.NoError(err)
	return theBot
}

// This test creates a chain between for peers: A<>B<>C<>D
// D publishes messages and they need to reach A before
func TestFeedsLiveSimpleFour(t *testing.T) {
	// <setup boilerplate>
	defer leakcheck.Check(t)

	r := require.New(t)
	a := assert.New(t)
	os.RemoveAll(filepath.Join("testrun", t.Name()))

	ctx, cancel := ShutdownContext(context.Background())
	botgroup, ctx := errgroup.WithContext(ctx)

	delayHist := gohistogram.NewHistogram(20)
	info := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, info)

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	netOpts := []Option{
		WithAppKey(appKey),
		WithContext(ctx),
		WithHMACSigning(hmacKey),
	}
	// </setup boilerplate>

	// create bots A to D
	botA := makeNamedTestBot(t, "A", append(netOpts, WithWebsocketAddress("localhost:12345")))
	botgroup.Go(bs.Serve(botA))
	t.Log("botA:", botA.KeyPair.ID().ShortSigil())

	botB := makeNamedTestBot(t, "B", netOpts)
	botgroup.Go(bs.Serve(botB))
	t.Log("botB:", botB.KeyPair.ID().ShortSigil())

	botC := makeNamedTestBot(t, "C", netOpts)
	botgroup.Go(bs.Serve(botC))
	t.Log("botC:", botC.KeyPair.ID().ShortSigil())

	botD := makeNamedTestBot(t, "D", netOpts)
	botgroup.Go(bs.Serve(botD))
	t.Log("botD:", botD.KeyPair.ID().ShortSigil())

	// replicate the network
	botA.Replicate(botB.KeyPair.ID())
	botA.Replicate(botC.KeyPair.ID())
	botA.Replicate(botD.KeyPair.ID())

	botB.Replicate(botA.KeyPair.ID())
	botB.Replicate(botC.KeyPair.ID())
	botB.Replicate(botD.KeyPair.ID())

	botC.Replicate(botA.KeyPair.ID())
	botC.Replicate(botB.KeyPair.ID())
	botC.Replicate(botD.KeyPair.ID())

	botD.Replicate(botA.KeyPair.ID())
	botD.Replicate(botB.KeyPair.ID())
	botD.Replicate(botC.KeyPair.ID())

	theBots := []*Sbot{botA, botB, botC, botD}

	feedOfBotD, err := botA.Users.Get(storedrefs.Feed(botD.KeyPair.ID()))
	r.NoError(err)

	t.Log("connecting the chain")
	// dial up A->B, B->C, C->D
	err = botA.Network.Connect(ctx, botB.Network.GetListenAddr())
	r.NoError(err)

	err = botB.Network.Connect(ctx, botC.Network.GetListenAddr())
	r.NoError(err)

	err = botC.Network.Connect(ctx, botD.Network.GetListenAddr())
	r.NoError(err)

	// setup live listener
	gotMsg := make(chan refs.Message)

	seqSrc, err := mutil.Indirect(botA.ReceiveLog, feedOfBotD).Query(
		margaret.Live(true),
	)
	r.NoError(err)

	botgroup.Go(makeChanWaiter(ctx, seqSrc, gotMsg))

	t.Log("starting live test")

	// now publish on D and let them bubble to A, live without reconnect
	timeouts := 0
	for i := 0; i < testMessageCount; i++ {
		rxSeq, err := botD.PublishLog.Append(refs.NewPost(fmt.Sprintf("test msg:%d", i)))
		r.NoError(err)
		published := time.Now()
		if !a.Equal(int64(i), rxSeq) {
			testutils.StreamLog(t, botD.ReceiveLog)
			break
		}

		// received new message?
		select {
		case <-time.After(time.Second / 2):
			t.Errorf("timeout %d....", i)
			timeouts++
		case msg, ok := <-gotMsg:
			r.True(ok, "%d: got msg", i)
			a.EqualValues(int64(i+1), msg.Seq(), "wrong seq on try %d", i)
			delayHist.Add(time.Since(published).Seconds())
		}
	}
	a.EqualValues(0, timeouts, "too many timeouts")

	cancel()
	time.Sleep(1 * time.Second)
	for _, bot := range theBots {
		err = bot.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err)
		bot.Shutdown()
		r.NoError(bot.Close())
	}
	r.NoError(botgroup.Wait())

	t.Log("cleanup complete")
	t.Log("delay mean:", time.Duration(delayHist.Mean()*float64(time.Second)))
	t.Log("delay variance:", time.Duration(delayHist.Variance()*float64(time.Second)))
}

// setup two bots, connect once and publish afterwards
func TestFeedsLiveSimpleTwo(t *testing.T) {
	// <setup boilerplate>
	defer leakcheck.Check(t)

	r := require.New(t)
	a := assert.New(t)

	ctx, cancel := ShutdownContext(context.TODO())

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	botgroup, ctx := errgroup.WithContext(ctx)

	mainLog := testutils.NewRelativeTimeLogger(nil)
	// </setup boilerplate>

	//setup the two bots
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "unit", "ali")),

		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
	)
	r.NoError(err)

	botgroup.Go(func() error {
		err := ali.Network.Serve(ctx)
		if err != nil {
			level.Warn(mainLog).Log("event", "ali serve exited", "err", err)
		}
		if errors.Is(err, ssb.ErrShuttingDown) {
			return nil
		}
		return err
	})

	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "unit", "bob")),

		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		WithListenAddr(":0"),
	)
	r.NoError(err)

	botgroup.Go(func() error {
		err := bob.Network.Serve(ctx)
		if err != nil {
			level.Warn(mainLog).Log("event", "bob serve exited", "err", err)
		}
		if errors.Is(err, ssb.ErrShuttingDown) {
			return nil
		}
		return err
	})

	// make them friends
	ali.Replicate(bob.KeyPair.ID())
	bob.Replicate(ali.KeyPair.ID())

	seq, err := ali.PublishLog.Append(refs.NewContactFollow(bob.KeyPair.ID()))
	r.NoError(err)
	r.Equal(int64(0), seq)

	seq, err = bob.PublishLog.Append(refs.NewContactFollow(ali.KeyPair.ID()))
	r.NoError(err)
	r.Equal(int64(0), seq)

	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(time.Second / 2)

	alisLog, err := bob.Users.Get(storedrefs.Feed(ali.KeyPair.ID()))
	r.NoError(err)

	wantSeq := int64(0)

	a.Equal(wantSeq, alisLog.Seq(), "after connect check")

	// setup live listener
	gotMsg := make(chan refs.Message)

	seqSrc, err := mutil.Indirect(bob.ReceiveLog, alisLog).Query(
		margaret.Gt(wantSeq),
		margaret.Live(true),
	)
	r.NoError(err)

	botgroup.Go(makeChanWaiter(ctx, seqSrc, gotMsg))

	if testing.Short() {
		testMessageCount = 25
	}
	for i := 0; i < testMessageCount; i++ {
		seq, err = ali.PublishLog.Append(refs.NewPost("first msg after connect"))
		r.NoError(err)
		r.Equal(int64(2+i), seq)

		// received new message?
		select {
		case <-time.After(2 * time.Second):
			t.Errorf("timeout %d....", i)
		case seq := <-gotMsg:
			a.EqualValues(int64(2+i), seq.Seq(), "wrong seq")
		}
	}

	a.EqualValues(int64(testMessageCount), alisLog.Seq(), "wrong seq")

	// cleanup
	err = ali.FSCK(FSCKWithMode(FSCKModeSequences))
	a.NoError(err, "FSCK error on A")
	err = bob.FSCK(FSCKWithMode(FSCKModeSequences))
	a.NoError(err, "FSCK error on B")

	cancel()
	ali.Shutdown()
	bob.Shutdown()

	r.NoError(ali.Close())
	r.NoError(bob.Close())

	r.NoError(botgroup.Wait())
}

// A publishes and is only connected to I
// B1 to N are connected to I and should get the fan out
func XTestFeedsLiveSimpleStar(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)
	os.RemoveAll(filepath.Join("testrun", t.Name()))

	ctx, cancel := ShutdownContext(context.Background())
	botgroup, ctx := errgroup.WithContext(ctx)

	info := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, info)

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	netOpts := []Option{
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		DisableEBT(true),
	}

	botA := makeNamedTestBot(t, "A", netOpts)
	botgroup.Go(bs.Serve(botA))

	// intermediary
	botI := makeNamedTestBot(t, "I", netOpts)
	botgroup.Go(bs.Serve(botI))

	var bLeafs []*Sbot
	for i := 0; i < 6; i++ {
		botBi := makeNamedTestBot(t, fmt.Sprintf("B%0d", i), netOpts)
		botgroup.Go(bs.Serve(botBi))
		bLeafs = append(bLeafs, botBi)
	}

	theBots := []*Sbot{botA, botI} // all the bots
	theBots = append(theBots, bLeafs...)

	// be-friend the network
	botA.Replicate(botI.KeyPair.ID())
	botI.Replicate(botA.KeyPair.ID())

	for _, bot := range bLeafs {

		// fetch the target
		bot.Replicate(botA.KeyPair.ID())

		// trust intermediary
		bot.Replicate(botI.KeyPair.ID())
		botI.Replicate(bot.KeyPair.ID())

	}

	var extraTestMessages = testMessageCount

	for n := extraTestMessages; n > 0; n-- {
		tMsg := refs.NewPost(fmt.Sprintf("some pre-setup msg %d", n))
		_, err := botA.PublishLog.Append(tMsg)
		r.NoError(err)
	}

	// note: assumes all bot's have the same message count
	initialSync(t, theBots, extraTestMessages)

	seqOfFeedA := int64(extraTestMessages) - 1 // N pre messages -1 for  0-indexed "array"/log
	var botBreceivedNewMessage []<-chan refs.Message
	for i, bot := range bLeafs {

		// did Bi get feed A?
		feedAonBotB, err := bot.Users.Get(storedrefs.Feed(botA.KeyPair.ID()))
		r.NoError(err)

		a.EqualValues(seqOfFeedA, feedAonBotB.Seq(), "botB%02d should have all of A's messages", i)

		// setup live listener
		gotMsg := make(chan refs.Message)

		seqSrc, err := mutil.Indirect(bot.ReceiveLog, feedAonBotB).Query(
			margaret.Gt(seqOfFeedA),
			margaret.Live(true),
		)
		r.NoError(err)

		botgroup.Go(makeChanWaiter(ctx, seqSrc, gotMsg))
		botBreceivedNewMessage = append(botBreceivedNewMessage, gotMsg)
	}

	t.Log("starting live test")
	// connect all bots to I
	for i, botX := range append(bLeafs, botA) {
		err := botX.Network.Connect(ctx, botI.Network.GetListenAddr())
		r.NoError(err, "connect bot%d>I failed", i)
	}

	timeouts := 0
	for i := 0; i < testMessageCount; i++ {
		tMsg := refs.NewPost(fmt.Sprintf("some fresh msg %d", i))
		seq, err := botA.PublishLog.Append(tMsg)
		r.NoError(err)
		// published := time.Now()
		r.EqualValues(extraTestMessages+i, seq, "new msg %d", i)

		// received new message?
		// TODO: reflect on slice of chans for less sleep

		wantSeq := int(seqOfFeedA+2) + i
		for bI, bChan := range botBreceivedNewMessage {
			select {
			case <-time.After(time.Second):
				t.Errorf("botB%02d: timeout on %d", bI, wantSeq)
				timeouts++
				if timeouts > 10 {
					t.Fatal("too many timeouts")
				}
			case msg := <-bChan:
				a.EqualValues(wantSeq, msg.Seq(), "botB%02d: wrong seq", bI)
				// t.Log("delay:", time.Since(published))
			}
		}
	}
	a.Equal(0, timeouts, "expected 0 timeouts")

	// cleanup
	time.Sleep(1 * time.Second)
	cancel()
	time.Sleep(1 * time.Second)
	for bI, bot := range append(bLeafs, botA, botI) {
		err := bot.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err, "botB%02d fsck", bI)
		bot.Shutdown()
		r.NoError(bot.Close(), "closed botB%02d failed", bI)
	}
	r.NoError(botgroup.Wait())
}

func initialSync(t testing.TB, theBots []*Sbot, expectedMsgCount int) {
	ctx := context.TODO()
	r := require.New(t)
	a := assert.New(t)

initialSync:
	for z := 3; z > 0; z-- {

		for bI, botX := range theBots {
			for bJ, botY := range theBots {
				if bI == bJ {
					continue
				}
				err := botX.Network.Connect(ctx, botY.Network.GetListenAddr())
				r.NoError(err)
			}

			time.Sleep(time.Second * 2) // settle sync
			var (
				complete = 0
				broken   = false
			)
			for i, bot := range theBots {

				if rootSeq := int(bot.ReceiveLog.Seq()); rootSeq == expectedMsgCount-1 {
					complete++
				} else {
					if rootSeq > expectedMsgCount-1 {
						err := bot.FSCK(FSCKWithMode(FSCKModeSequences))
						if err != nil {
							broken = true
							t.Error(err)
						}
						t.Fatal("bot", i, "has more messages then expected:", rootSeq)
					}
					t.Log("init sync delay on bot", i, ": seq", rootSeq)
				}
			}
			if broken {
				t.Fatal()
			}
			if len(theBots) == complete {
				t.Log("initsync done")
				break initialSync
			}
			botX.Network.GetConnTracker().CloseAll()
			t.Log("continuing initialSync.. bots complete:", complete)
		}
	}

	var failed bool
	// check fsck,sequences and and disconnect
	for i, bot := range theBots {

		if !a.EqualValues(expectedMsgCount, bot.ReceiveLog.Seq()+1, "wrong rxSeq on bot %d", i) {
			failed = true
		}
		err := bot.FSCK(FSCKWithMode(FSCKModeSequences))
		r.NoError(err, "FSCK error on bot %d", i)
		ct := bot.Network.GetConnTracker()
		ct.CloseAll()
		time.Sleep(1 * time.Second)
		r.EqualValues(ct.Count(), 0, "%d still has connectons", i)
	}

	if failed {
		t.Fatal("initial replication failed")
	}
}

func makeChanWaiter(ctx context.Context, src luigi.Source, gotMsg chan<- refs.Message) func() error {
	return func() error {
		defer close(gotMsg)
		for {
			v, err := src.Next(ctx)
			if err != nil {
				if luigi.IsEOS(err) || errors.Is(err, ssb.ErrShuttingDown) {
					return nil
				}
				return err
			}

			msg := v.(refs.Message)

			// fmt.Println("rxFeed", msg.Author().Ref()[1:5], "msgSeq", msg.Seq(), "key", msg.Key().Ref())
			select {
			case gotMsg <- msg:

			case <-ctx.Done():
				return nil
			}
		}
	}
}
