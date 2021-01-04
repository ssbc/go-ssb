// SPDX-License-Identifier: MIT

package private_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestPrivatePublish(t *testing.T) {
	t.Run("classic", testPublishPerAlgo(refs.RefAlgoFeedSSB1))
	t.Run("gabby", testPublishPerAlgo(refs.RefAlgoFeedGabby))
}

func testPublishPerAlgo(algo string) func(t *testing.T) {
	return func(t *testing.T) {
		r, a := require.New(t), assert.New(t)

		srvRepo := filepath.Join("testrun", t.Name(), "serv")
		os.RemoveAll(srvRepo)

		alice, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("alice"), 8)))
		r.NoError(err)
		alice.Id.Algo = algo

		srvLog := kitlog.NewNopLogger()
		if testing.Verbose() {
			srvLog = kitlog.NewJSONLogger(os.Stderr)
		}

		srv, err := sbot.New(
			sbot.WithKeyPair(alice),
			sbot.WithInfo(srvLog),
			sbot.WithRepoPath(srvRepo),
			sbot.WithListenAddr(":0"),
			sbot.LateOption(sbot.WithUNIXSocket()),
		)
		r.NoError(err, "failed to init sbot")

		const n = 32
		for i := n; i > 0; i-- {
			_, err := srv.PublishLog.Publish(struct {
				Type string `json:"type"`
				Text string
				I    int
			}{"test", "clear text!", i})
			r.NoError(err)
		}

		r.NoError(err, "sbot srv init failed")

		c, err := client.NewUnix(filepath.Join(srvRepo, "socket"))
		r.NoError(err, "failed to make client connection")

		type msg struct {
			Type string `json:"type"`
			Msg  string
		}
		ref, err := c.PrivatePublish(msg{"test", "hello, world"}, alice.Id)
		r.NoError(err, "failed to publish")
		r.NotNil(ref)

		src, err := c.PrivateRead()
		r.NoError(err, "failed to open private stream")

		v, err := src.Next(context.TODO())
		r.NoError(err, "failed to get msg")

		savedMsg, ok := v.(refs.Message)
		r.True(ok, "wrong type: %T", v)
		if !a.Equal(savedMsg.Key().Ref(), ref.Ref()) {

			whoops, err := srv.Get(*ref)
			r.NoError(err)
			t.Log(string(whoops.ContentBytes()))
		}

		v, err = src.Next(context.TODO())
		r.Error(err)
		r.True(errors.Is(err, luigi.EOS{}))

		// try with seqwrapped query
		pl, ok := srv.GetMultiLog(multilogs.IndexNamePrivates)
		r.True(ok)

		userPrivs, err := pl.Get(librarian.Addr("box1:") + storedrefs.Feed(srv.KeyPair.Id))
		r.NoError(err)

		unboxlog := private.NewUnboxerLog(srv.ReceiveLog, userPrivs, srv.KeyPair)

		src, err = unboxlog.Query(margaret.SeqWrap(true))
		r.NoError(err)

		v, err = src.Next(context.TODO())
		r.NoError(err, "failed to get msg")

		sw, ok := v.(margaret.SeqWrapper)
		r.True(ok, "wrong type: %T", v)
		wrappedVal := sw.Value()
		savedMsg, ok = wrappedVal.(refs.Message)
		r.True(ok, "wrong type: %T", wrappedVal)
		r.Equal(savedMsg.Key().Ref(), ref.Ref())

		v, err = src.Next(context.TODO())
		r.Error(err)
		r.True(errors.Is(err, luigi.EOS{}))

		// shutdown
		a.NoError(c.Close())
		srv.Shutdown()
		r.NoError(srv.Close())
	}
}
