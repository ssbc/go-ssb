// SPDX-License-Identifier: MIT

package sbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2/debug"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
)

func TestFeedsOneByOne(t *testing.T) {
	defer leakcheck.Check(t)
	t.Run("legacy", createFeedsOneByOneTest(false))
	t.Run("ebt", createFeedsOneByOneTest(true))
}

func createFeedsOneByOneTest(useEBT bool) func(t *testing.T) {
	return func(t *testing.T) {
		r := require.New(t)
		a := assert.New(t)
		ctx, cancel := ShutdownContext(context.TODO())

		os.RemoveAll("testrun")

		appKey := make([]byte, 32)
		rand.Read(appKey)
		hmacKey := make([]byte, 32)
		rand.Read(hmacKey)

		seed := bytes.Repeat([]byte{1}, 32)
		aliKey, err := ssb.NewKeyPair(bytes.NewReader(seed), refs.RefAlgoFeedSSB1)
		r.NoError(err)
		t.Log("ali is", aliKey.ID().Ref())

		botgroup, ctx := errgroup.WithContext(ctx)

		mainLog := log.NewNopLogger()
		if testing.Verbose() {
			mainLog = testutils.NewRelativeTimeLogger(nil)
			// comment this out for even more verbosity
			mainLog = level.NewFilter(mainLog, level.AllowInfo())
		}

		aliPath := filepath.Join("testrun", t.Name(), "ali")
		ali, err := New(
			WithAppKey(appKey),
			WithHMACSigning(hmacKey),
			WithContext(ctx),
			WithKeyPair(aliKey),
			WithInfo(log.With(mainLog, "unit", "ali")),
			WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
				return debug.WrapDump(filepath.Join(aliPath, "muxdump"), conn)
			}),
			WithRepoPath(aliPath),
			WithListenAddr(":0"),
			DisableEBT(!useEBT),
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

		seed = bytes.Repeat([]byte{2}, 32)
		bobKey, err := ssb.NewKeyPair(bytes.NewReader(seed), refs.RefAlgoFeedSSB1)
		r.NoError(err)
		t.Log("bob is", bobKey.ID().Ref())

		bobPath := filepath.Join("testrun", t.Name(), "bob")
		bob, err := New(
			WithAppKey(appKey),
			WithHMACSigning(hmacKey),
			WithContext(ctx),
			WithKeyPair(bobKey),
			WithInfo(log.With(mainLog, "unit", "bob")),
			WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
				return debug.WrapDump(filepath.Join(bobPath, "muxdump"), conn)
			}),
			WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
			WithListenAddr(":0"),
			DisableEBT(!useEBT),
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

		ali.Replicate(bob.KeyPair.ID())
		bob.Replicate(ali.KeyPair.ID())

		uf, ok := bob.GetMultiLog("userFeeds")
		r.True(ok)

		alisLog, err := uf.Get(storedrefs.Feed(ali.KeyPair.ID()))
		r.NoError(err)

		n := 15
		if testing.Short() {
			n = 3
		}

		for i := 0; i < n; i++ {
			newSeq, err := ali.PublishLog.Append(map[string]interface{}{
				"type": "test-value",
				"test": i,
			})
			r.NoError(err)
			t.Log("published", newSeq)

			t.Logf("connecting (%d)", i)
			err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
			r.NoError(err)
			time.Sleep(250 * time.Millisecond)
			bob.Network.GetConnTracker().CloseAll()

			// check the note is updated correctly
			if useEBT {
				aliHas, err := bob.ebtState.Inspect(ali.KeyPair.ID())
				r.NoError(err)
				alisNoteAtBob := aliHas[ali.KeyPair.ID().Ref()]
				a.True(int(alisNoteAtBob.Seq) >= i, "expected more messages have %d", alisNoteAtBob.Seq)
			}

			a.Equal(int64(i), alisLog.Seq(), "check run %d", i)
		}

		err = ali.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err, "fsck on A failed")
		err = bob.FSCK(FSCKWithMode(FSCKModeSequences))
		a.NoError(err, "fsck on B failed")

		cancel()
		ali.Shutdown()
		bob.Shutdown()

		r.NoError(ali.Close())
		r.NoError(bob.Close())

		r.NoError(botgroup.Wait())
	}
}
