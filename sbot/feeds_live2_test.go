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
	"strconv"
	"testing"
	"time"

	"github.com/VividCortex/gohistogram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
)

func TestFeedsLiveNetworkChain(t *testing.T) {
	t.Run("len3", makeFeedsLiveNetworkChain(3))
	if testing.Short() {
		return
	}
	t.Run("len5", makeFeedsLiveNetworkChain(5))
	t.Run("len7", makeFeedsLiveNetworkChain(7))
	t.Run("len9", makeFeedsLiveNetworkChain(9))
}

func makeFeedsLiveNetworkChain(chainLen uint) func(t *testing.T) {
	return func(t *testing.T) {
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

		theBots := []*Sbot{}
		n := int(chainLen)
		for i := 0; i < n; i++ {
			botI := makeNamedTestBot(t, strconv.Itoa(i), netOpts)
			botgroup.Go(bs.Serve(botI))
			theBots = append(theBots, botI)
		}

		// all one expect diagonal
		followMatrix := make([]int, n*n)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i == j {
					continue
				}
				x := i*n + j
				followMatrix[x] = 1
			}
		}

		msgCnt := 0
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				x := i*n + j
				fQ := followMatrix[x]

				botI := theBots[i]
				botJ := theBots[j]

				if fQ == 1 {
					msgCnt++
					botI.Replicate(botJ.KeyPair.ID())
					_, err := botI.PublishLog.Append(refs.NewContactFollow(botJ.KeyPair.ID()))
					r.NoError(err)
				}
			}
		}

		initialSync(t, theBots, msgCnt)

		// dial up a chain
		for i := 0; i < n-1; i++ {
			botI := theBots[i]
			botJ := theBots[i+1]

			err := botI.Network.Connect(ctx, botJ.Network.GetListenAddr())
			r.NoError(err)
		}
		time.Sleep(1 * time.Second)

		// did b0 get feed of bN-1?
		feedIndexOfBot0, ok := theBots[0].GetMultiLog("userFeeds")
		r.True(ok)
		feedOfLastBot, err := feedIndexOfBot0.Get(storedrefs.Feed(theBots[n-1].KeyPair.ID()))
		r.NoError(err)
		wantSeq := int64(n - 2)
		r.EqualValues(wantSeq, feedOfLastBot.Seq(), "after connect check")

		// setup live listener
		gotMsg := make(chan refs.Message)

		seqSrc, err := mutil.Indirect(theBots[0].ReceiveLog, feedOfLastBot).Query(
			margaret.Gt(wantSeq),
			margaret.Live(true),
		)
		r.NoError(err)

		botgroup.Go(makeChanWaiter(ctx, seqSrc, gotMsg))

		// now publish on C and let them bubble to A, live without reconnect
		for i := 0; i < testMessageCount; i++ {
			tmsg := refs.NewPost(fmt.Sprintf("some test msg:%02d", n))
			rxSeq, err := theBots[n-1].PublishLog.Append(tmsg)
			r.NoError(err)
			published := time.Now()
			a.EqualValues(int64(msgCnt+i), rxSeq)

			// received new message?
			select {
			case <-time.After(2 * time.Second):
				t.Errorf("timeout %d....", i)
			case msg := <-gotMsg:
				a.EqualValues(int64(n+i), msg.Seq(), "wrong seq")
				delayHist.Add(time.Since(published).Seconds())
			}
		}

		// cleanup
		cancel()
		time.Sleep(1 * time.Second)
		for bI, bot := range theBots {
			err = bot.FSCK(FSCKWithMode(FSCKModeSequences))
			a.NoError(err, "bot%02d fsck", bI)
			bot.Shutdown()
			r.NoError(bot.Close(), "failed to close bot%02d fsck", bI)
		}
		r.NoError(botgroup.Wait())

		t.Log("cleanup complete")
		t.Log("delay mean:", time.Duration(delayHist.Mean()*float64(time.Second)))
		t.Log("delay variance:", time.Duration(delayHist.Variance()*float64(time.Second)))
	}
}

func TestFeedsLiveNetworkStar(t *testing.T) {
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
		WithContext(ctx),
		WithHMACSigning(hmacKey),
	}

	botA := makeNamedTestBot(t, "A", netOpts)
	botgroup.Go(bs.Serve(botA))

	botB := makeNamedTestBot(t, "B", netOpts)
	botgroup.Go(bs.Serve(botB))

	botC := makeNamedTestBot(t, "C", netOpts)
	botgroup.Go(bs.Serve(botC))

	theBots := []*Sbot{botA, botB, botC}

	followMatrix := []int{
		0, 1, 1,
		1, 0, 1,
		1, 1, 0,
	}

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			x := i*3 + j
			fQ := followMatrix[x]

			botI := theBots[i]
			botJ := theBots[j]

			if fQ == 1 {
				botI.Replicate(botJ.KeyPair.ID())
				_, err := botI.PublishLog.Append(refs.NewContactFollow(botJ.KeyPair.ID()))
				r.NoError(err)
			}
		}
	}

	initialSync(t, theBots, 6)

	// dial up A->B and B->C
	err := botA.Network.Connect(ctx, botB.Network.GetListenAddr())
	r.NoError(err)
	err = botB.Network.Connect(ctx, botC.Network.GetListenAddr())
	r.NoError(err)

	time.Sleep(3 / 2 * time.Second)

	// did B get feed C?
	ufOfBotB, ok := botB.GetMultiLog("userFeeds")
	r.True(ok)
	feedOfBotCAtB, err := ufOfBotB.Get(storedrefs.Feed(botC.KeyPair.ID()))
	r.NoError(err)

	wantSeq := int64(1)
	r.EqualValues(wantSeq, feedOfBotCAtB.Seq(), "after connect check")

	t.Log("commencing live tests")

	// setup listener
	uf, ok := botA.GetMultiLog("userFeeds")
	r.True(ok)
	feedOfBotC, err := uf.Get(storedrefs.Feed(botC.KeyPair.ID()))
	r.NoError(err)

	gotMsg := make(chan refs.Message)

	seqSrc, err := mutil.Indirect(botA.ReceiveLog, feedOfBotC).Query(
		margaret.Gt(wantSeq),
		margaret.Live(true))
	r.NoError(err)

	botgroup.Go(makeChanWaiter(ctx, seqSrc, gotMsg))

	// now publish on C and let them bubble to A, live without reconnect
	timeouts := 0
	for i := 0; i < testMessageCount; i++ {
		rxSeq, err := botC.PublishLog.Append(refs.NewPost("some test msg"))
		r.NoError(err)
		r.Equal(int64(6+i), rxSeq)
		//t.Log(rxSeq)

		// received new message?
		select {
		case <-time.After(2 * time.Second):
			t.Errorf("timeout %d....", i)
			timeouts++
		case msg := <-gotMsg:
			a.EqualValues(int64(3+i), msg.Seq(), "wrong message seq")
		}
	}
	a.Equal(0, timeouts, "expected no timeouts")

	// cleanup
	cancel()
	time.Sleep(1 * time.Second)
	for _, bot := range theBots {
		err = bot.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err)
		bot.Shutdown()
		r.NoError(bot.Close())
	}
	r.NoError(botgroup.Wait())
}

func XTestFeedsLiveNetworkDiamond(t *testing.T) {
	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
	}
	r := require.New(t)
	a := assert.New(t)
	os.RemoveAll(filepath.Join("testrun", t.Name()))

	ctx, cancel := ShutdownContext(context.Background())
	botgroup, ctx := errgroup.WithContext(ctx)

	delayHist := gohistogram.NewHistogram(5)
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
		WithInfo(info),
		WithHops(3),
	}

	theBots := []*Sbot{}
	for n := 0; n < 6; n++ {
		botN := makeNamedTestBot(t, strconv.Itoa(n), netOpts)
		botgroup.Go(bs.Serve(botN))
		theBots = append(theBots, botN)
	}

	followMatrix := []int{
		0, 1, 1, 0, 0, 1,
		1, 0, 1, 1, 1, 0,
		1, 1, 0, 1, 1, 0,
		0, 1, 1, 0, 1, 1,
		0, 1, 1, 1, 0, 1,
		1, 0, 0, 1, 1, 0,
	}
	followMsgs := 0
	for i := 0; i < 6; i++ {
		for j := 0; j < 6; j++ {

			x := i*6 + j
			fQ := followMatrix[x]

			botI := theBots[i]
			botJ := theBots[j]

			if fQ == 1 {
				botI.Replicate(botJ.KeyPair.ID())
				msg, err := botI.PublishLog.Publish(refs.NewContactFollow(botJ.KeyPair.ID()))
				r.NoError(err)
				t.Log(i, "followed", j, msg.Key().ShortSigil())
				followMsgs++
			}
		}
	}

	// let the graph walk trigger
	// TODO: have an api for that
	time.Sleep(5 * time.Second)

	bot0graph, err := theBots[0].GraphBuilder.Build()
	r.NoError(err)
	r.True(bot0graph.Follows(theBots[0].KeyPair.ID(), theBots[1].KeyPair.ID()))

	initialSync(t, theBots, followMsgs)

	// setup connections
	connectMatrix := []int{
		0, 1, 0, 0, 0, 0,
		0, 0, 1, 0, 1, 0,
		0, 0, 0, 1, 0, 0,
		0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0,
	}

	for i := 0; i < 6; i++ {
		for j := 0; j < 6; j++ {

			x := i*6 + j
			fQ := connectMatrix[x]

			botI := theBots[i]
			botJ := theBots[j]

			if fQ == 1 {
				err := botI.Network.Connect(ctx, botJ.Network.GetListenAddr())
				r.NoError(err)
				// t.Log(i, "connected", j)
				time.Sleep(1 * time.Second)
			}
		}
	}

	// setup live listener
	gotMsg := make(chan refs.Message)

	// construct query source
	feedOfBotC, err := theBots[0].Users.Get(storedrefs.Feed(theBots[5].KeyPair.ID()))
	r.NoError(err)
	seqSrc, err := mutil.Indirect(theBots[0].ReceiveLog, feedOfBotC).Query(
		// margaret.Gte(int64(3)),
		margaret.Live(true))
	r.NoError(err)

	botgroup.Go(makeChanWaiter(ctx, seqSrc, gotMsg))

	timeout := 0
	// now publish on C and let them bubble to A, live without reconnect
	for i := 0; i < testMessageCount; i++ {
		tMsg := refs.NewPost(fmt.Sprintf("some test msg %d", i))
		seq, err := theBots[5].PublishLog.Append(tMsg)
		r.NoError(err)
		published := time.Now()
		r.EqualValues(i, seq, "new msg %d", i)

		// received new message?
		select {
		case <-time.After(3 * time.Second):
			t.Errorf("timeout %d....", i)
			timeout++
			if timeout >= 5 {
				t.Fatal("too many timeouts")
			}
		case msg := <-gotMsg:
			a.EqualValues(int64(i+1), msg.Seq(), "wrong seq")
			delayHist.Add(time.Since(published).Seconds())
		}
	}

	// cleanup
	for i, bot := range theBots {
		err = bot.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err, "fsck of bot %d failed", i)
	}
	cancel()
	for _, bot := range theBots {
		bot.Shutdown()
		r.NoError(bot.Close())
	}
	r.NoError(botgroup.Wait())
	t.Log("cleanup complete")
	t.Log("delay mean:", time.Duration(delayHist.Mean()*float64(time.Second)))
	t.Log("delay variance:", time.Duration(delayHist.Variance()*float64(time.Second)))
}
