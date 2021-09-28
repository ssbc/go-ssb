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
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"
)

// store some feeds, stop the bot and reopen the repo
func XTestPersistence(t *testing.T) {
	defer leakcheck.Check(t)

	var err error

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
	t.Log("botA:", botA.KeyPair.ID().ShortSigil())

	botB := makeNamedTestBot(t, "B", netOpts)
	botgroup.Go(bs.Serve(botB))
	t.Log("botB:", botB.KeyPair.ID().ShortSigil())

	botC := makeNamedTestBot(t, "C", netOpts)
	botgroup.Go(bs.Serve(botC))
	t.Log("botC:", botC.KeyPair.ID().ShortSigil())

	// replicate the network
	botA.Replicate(botB.KeyPair.ID())
	botA.Replicate(botC.KeyPair.ID())

	botB.Replicate(botA.KeyPair.ID())
	botB.Replicate(botC.KeyPair.ID())

	botC.Replicate(botA.KeyPair.ID())
	botC.Replicate(botB.KeyPair.ID())

	theBots := []*Sbot{botA, botB, botC}

	// let them all publish some test messages
	testMsgCount := 50
	for ib, b := range theBots {
		for n := testMsgCount; n > 0; n-- {
			_, err = b.PublishLog.Publish(refs.NewPost(fmt.Sprintf("hello no %d from %d", n, ib)))
			r.NoError(err)
		}
	}

	// note: assumes all bot's have the same message count
	initialSync(t, theBots, testMsgCount*len(theBots))

	t.Log("connecting the chain")
	// dial up A->B, B->C
	err = botA.Network.Connect(ctx, botB.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)
	err = botB.Network.Connect(ctx, botC.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)

	cancel()
	time.Sleep(1 * time.Second)
	for _, bot := range theBots {
		err = bot.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err)
		bot.Shutdown()
		r.NoError(bot.Close())
	}
	r.NoError(botgroup.Wait())

	// all closed. now restart A

	mainLog := log.NewNopLogger()
	if testing.Verbose() {
		mainLog = testutils.NewRelativeTimeLogger(nil)
	}
	name := "A"
	testPath := filepath.Join("testrun", t.Name(), "bot-"+name)

	botOptions := append(netOpts,
		WithKeyPair(botA.KeyPair),
		WithInfo(log.With(mainLog, "bot", name)),
		WithRepoPath(testPath),
		DisableNetworkNode(),
	)

	botA, err = New(botOptions...)
	r.NoError(err)

	feeds, err := botA.Users.List()
	r.NoError(err)
	r.Len(feeds, 3)

	checkLogSeq := func(l margaret.Log) {
		r.EqualValues(testMsgCount-1, l.Seq())
	}

	logA, err := botA.Users.Get(storedrefs.Feed(botA.KeyPair.ID()))
	r.NoError(err)
	checkLogSeq(logA)

	logB, err := botA.Users.Get(storedrefs.Feed(botB.KeyPair.ID()))
	r.NoError(err)
	checkLogSeq(logB)

	logC, err := botA.Users.Get(storedrefs.Feed(botC.KeyPair.ID()))
	r.NoError(err)
	checkLogSeq(logC)

	r.NoError(botA.Close())
}
