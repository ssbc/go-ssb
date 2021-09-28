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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
)

func XTestFeedsLiveReconnect(t *testing.T) {
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
		DisableEBT(true),
	}

	botA := makeNamedTestBot(t, "A", netOpts)
	botgroup.Go(bs.Serve(botA))

	botI := makeNamedTestBot(t, "I", netOpts)
	botgroup.Go(bs.Serve(botI))

	var (
		bLeafs            []*Sbot
		extraTestMessages = 256
		extraBots         = 5
	)
	if testing.Short() {
		extraTestMessages = 25
		extraBots = 2
	}
	for i := 0; i < extraBots; i++ {
		botBi := makeNamedTestBot(t, fmt.Sprintf("B%0d", i), netOpts)
		botgroup.Go(bs.Serve(botBi))
		bLeafs = append(bLeafs, botBi)
	}

	theBots := []*Sbot{botA, botI} // all the bots
	theBots = append(theBots, bLeafs...)

	// be-friend the network
	_, err := botA.PublishLog.Append(refs.NewContactFollow(botI.KeyPair.ID()))
	r.NoError(err)
	botA.Replicate(botI.KeyPair.ID())
	_, err = botI.PublishLog.Append(refs.NewContactFollow(botA.KeyPair.ID()))
	botI.Replicate(botA.KeyPair.ID())
	r.NoError(err)
	var msgCnt = 2

	for i, bot := range bLeafs {
		_, err := bot.PublishLog.Append(refs.NewContactFollow(botI.KeyPair.ID()))
		r.NoError(err, "follow b%d>I failed", i)
		_, err = botI.PublishLog.Append(refs.NewContactFollow(bot.KeyPair.ID()))
		r.NoError(err, "follow I>b%d failed", i)
		msgCnt += 2

		bot.Replicate(botI.KeyPair.ID())
		botI.Replicate(bot.KeyPair.ID())

		// simulate hops
		bot.Replicate(botA.KeyPair.ID())
		botA.Replicate(bot.KeyPair.ID())
		for _, botB := range bLeafs {
			if botB == bot {
				continue
			}
			botB.Replicate(bot.KeyPair.ID())
		}
	}

	msgCnt += extraTestMessages
	for n := extraTestMessages; n > 0; n-- {
		tMsg := fmt.Sprintf("some pre-setup msg %d", n)
		_, err := botA.PublishLog.Append(refs.NewPost(tMsg))
		r.NoError(err)
	}

	initialSync(t, theBots, msgCnt)

	seqOfFeedA := int64(extraTestMessages) // N pre messages +1 contact (0 indexed)

	// did Bi get feed A?
	botB0 := bLeafs[0]
	feedIdxOfB0, ok := botB0.GetMultiLog("userFeeds")
	r.True(ok)
	feedAonBotB, err := feedIdxOfB0.Get(storedrefs.Feed(botA.KeyPair.ID()))
	r.NoError(err)

	a.EqualValues(seqOfFeedA, feedAonBotB.Seq(), "botB0 should have all of A's messages")

	// setup live listener
	liveQry, err := mutil.Indirect(botB0.ReceiveLog, feedAonBotB).Query(
		margaret.Gt(seqOfFeedA),
		margaret.Live(true),
	)
	r.NoError(err)

	t.Log("starting live test")
	errs := 0
	// connect all bots to I
	for i, botX := range append(bLeafs, botA) {
		err := botX.Network.Connect(ctx, botI.Network.GetListenAddr())
		r.NoError(err, "connect bot%d>I failed", i)
	}
	for i := 0; i < extraTestMessages; i++ {
		tMsg := fmt.Sprintf("some fresh msg %d", i)
		seq, err := botA.PublishLog.Append(refs.NewPost(tMsg))
		r.NoError(err)
		r.EqualValues(msgCnt+i, seq, "new msg %d", i)

		if i%9 == 0 { // simulate faulty network
			botX := i%(extraBots-1) + 1 // some of the other (b[0] keeps receiveing)
			dcBot := bLeafs[botX]
			ct := dcBot.Network.GetConnTracker()
			ct.CloseAll()
			t.Log("disconnecting", botX)
			go func(b *Sbot) {
				time.Sleep(time.Second / 4)
				timeoutCtx, connectCancel := context.WithTimeout(ctx, 1*time.Minute)
				defer connectCancel()
				err := b.Network.Connect(timeoutCtx, botI.Network.GetListenAddr())
				r.NoError(err)
			}(dcBot)
		}

		// received new message?
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
		v, err := liveQry.Next(timeoutCtx)
		cancel()
		if err != nil {
			t.Error("liveQry err", err)
			errs++
			if errs > 3 {
				t.Fatal("too many errors")
			}
			continue
		}

		msg, ok := v.(refs.Message)
		r.True(ok, "got %T", v)

		a.EqualValues(int(seqOfFeedA+2)+i, msg.Seq(), "botB0: wrong seq")
	}

	time.Sleep(time.Second * 3)
	cancel()

	finalWantSeq := seqOfFeedA + int64(extraTestMessages)
	for i, bot := range bLeafs {

		// did Bi get feed A?
		ufOfBotB, ok := bot.GetMultiLog("userFeeds")
		r.True(ok)

		feedAonBotB, err := ufOfBotB.Get(storedrefs.Feed(botA.KeyPair.ID()))
		r.NoError(err)

		a.EqualValues(finalWantSeq, feedAonBotB.Seq(), "botB%02d should have all of A's messages", i)
	}

	// cleanup
	time.Sleep(1 * time.Second)
	for bI, bot := range append(bLeafs, botA, botI) {
		err = bot.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err, "botB%02d fsck", bI)
		bot.Shutdown()
		r.NoError(bot.Close(), "closed botB%02d failed", bI)
	}
	r.NoError(botgroup.Wait())
}
