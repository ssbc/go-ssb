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

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.mindeco.de/log"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message/multimsg"
	refs "go.mindeco.de/ssb-refs"
)

func TestFeedsFormatSupport(t *testing.T) {
	formats := []refs.RefAlgo{
		refs.RefAlgoFeedSSB1,
		refs.RefAlgoFeedBendyButt,
		refs.RefAlgoFeedGabby,
	}
	for _, format := range formats {
		t.Run(fmt.Sprintf("%s/legacy", format), testGenericFeedFormatSync(format, false))
		t.Run(fmt.Sprintf("%s/ebt", format), testGenericFeedFormatSync(format, true))
	}
}

func testGenericFeedFormatSync(format refs.RefAlgo, withEBT bool) func(t *testing.T) {
	return func(t *testing.T) {
		// <boilerplate>
		defer leakcheck.Check(t)
		r := require.New(t)

		ctx, cancel := ShutdownContext(context.Background())
		botgroup, ctx := errgroup.WithContext(ctx)

		info := testutils.NewRelativeTimeLogger(nil)
		bs := newBotServer(ctx, info)

		tPath := filepath.Join("testrun", t.Name())
		os.RemoveAll(tPath)

		appKey := make([]byte, 32)
		rand.Read(appKey)
		hmacKey := make([]byte, 32)
		rand.Read(hmacKey)
		mainLog := testutils.NewRelativeTimeLogger(nil)
		// </boilerplate>

		// create the two bots
		ali, err := New(
			WithAppKey(appKey),
			WithHMACSigning(hmacKey),
			WithContext(ctx),
			WithInfo(log.With(mainLog, "unit", "ali")),
			WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
			WithListenAddr(":0"),
			DisableEBT(!withEBT),
		)
		r.NoError(err)
		botgroup.Go(bs.Serve(ali))

		// bob is the one with the other feed format
		bobsKey, err := ssb.NewKeyPair(nil, format)
		r.NoError(err)

		bob, err := New(
			WithAppKey(appKey),
			WithHMACSigning(hmacKey),
			WithContext(ctx),
			WithKeyPair(bobsKey),
			WithInfo(log.With(mainLog, "unit", "bob")),
			WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
			WithListenAddr(":0"),
			DisableEBT(!withEBT),
		)
		r.NoError(err)
		botgroup.Go(bs.Serve(bob))

		// be friends
		ali.Replicate(bob.KeyPair.ID())
		bob.Replicate(ali.KeyPair.ID())

		seq, err := ali.PublishLog.Append(refs.NewContactFollow(bob.KeyPair.ID()))
		r.NoError(err)
		r.Equal(int64(0), seq)

		seq, err = bob.PublishLog.Append(refs.NewContactFollow(ali.KeyPair.ID()))
		r.NoError(err)
		r.Equal(int64(0), seq)

		// bob publishes his test messages
		for i := 0; i < 9; i++ {
			seq, err := bob.PublishLog.Append(map[string]interface{}{
				"type": "test",
				"test": i,
			})
			r.NoError(err)
			r.Equal(int64(i+1), seq)
		}

		// sanity, check bob has his shit together
		bobsOwnLog, err := bob.Users.Get(storedrefs.Feed(bob.KeyPair.ID()))
		r.NoError(err)
		r.Equal(int64(9), bobsOwnLog.Seq(), "bob doesn't have his own log!")

		// dial
		err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
		r.NoError(err)

		// give time to sync
		time.Sleep(3 * time.Second)
		// be done
		ali.Network.GetConnTracker().CloseAll()

		// check that bobs messages got to ali
		bosLogAtAli, err := ali.Users.Get(storedrefs.Feed(bob.KeyPair.ID()))
		r.NoError(err)

		r.Equal(int64(9), bosLogAtAli.Seq())

		src, err := mutil.Indirect(ali.ReceiveLog, bosLogAtAli).Query()
		r.NoError(err)
		for {
			v, err := src.Next(ctx)
			if luigi.IsEOS(err) {
				break
			} else if err != nil {
				r.NoError(err)
			}
			msg, ok := v.(*multimsg.MultiMessage)
			r.True(ok, "Type: %T", v)

			r.Equal(format, msg.Author().Algo(), "wrong format")
		}

		// shutdown and cleanup
		cancel()
		ali.Shutdown()
		bob.Shutdown()
		time.Sleep(1 * time.Second)
		r.NoError(ali.Close())
		r.NoError(bob.Close())

		r.NoError(botgroup.Wait())
	}
}
