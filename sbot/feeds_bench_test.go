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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"
)

func benchChain(chainLen int) func(b *testing.B) {
	return func(b *testing.B) {
		r := require.New(b)
		a := assert.New(b)

		b.StopTimer()

		for n := 0; n < b.N; n++ {
			os.RemoveAll(filepath.Join("testrun", b.Name()))
			ctx, cancel := context.WithCancel(context.TODO())
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
			}

			theBots := []*Sbot{}
			n := int(chainLen)
			for i := 0; i < n; i++ {
				botI := makeNamedTestBot(b, strconv.Itoa(i), netOpts)
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
						_, err := botI.PublishLog.Publish(refs.NewContactFollow(botJ.KeyPair.ID()))
						r.NoError(err)
					}
				}
			}

			initialSync(b, theBots, msgCnt)

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

			seqSrc, err := mutil.Indirect(theBots[0].ReceiveLog, feedOfLastBot).Query(
				margaret.Gt(wantSeq),
				margaret.Live(true),
			)
			r.NoError(err)

			b.StartTimer()
			// now publish on C and let them bubble to A, live without reconnect
			for i := 0; i < testMessageCount; i++ {
				rxSeq, err := theBots[n-1].PublishLog.Append(fmt.Sprintf("some test msg:%02d", n))
				r.NoError(err)
				a.EqualValues(int64(msgCnt+i), rxSeq)

				timeoutCtx, done := context.WithTimeout(ctx, 2*time.Second)

				v, err := seqSrc.Next(timeoutCtx)
				r.NoError(err)
				msg, ok := v.(refs.Message)
				r.True(ok)
				a.EqualValues(int64(n+i), msg.Seq(), "wrong seq")
				done()
			}
			b.StopTimer()

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
		}
	}
}

func BenchmarkChain(b *testing.B) {
	b.Run("2", benchChain(2))
	// b.Run("5", benchChain(5))
	// b.Run("7", benchChain(7))
}
