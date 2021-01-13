// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestEncodeHistStreamAsJSON(t *testing.T) {
	// defer leakcheck.Check(t)
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)

	srvLog := testutils.NewRelativeTimeLogger(nil)

	testKP, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	testKP.Id.Algo = refs.RefAlgoFeedGabby

	srv, err := sbot.New(
		sbot.WithKeyPair(testKP),
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"))
	r.NoError(err, "sbot srv init failed")

	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(context.TODO())
		if err != nil {
			srvErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(srvErrc)
	}()

	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(testKP, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	r.True(muxrpc.IsServer(c.Endpoint), "should be talking to a server")

	// no messages yet
	seqv, err := srv.ReceiveLog.Seq().Value()
	r.NoError(err, "failed to get root log sequence")
	r.Equal(margaret.SeqEmpty, seqv)

	var wantRefs []string
	for i := 0; i < 10; i++ {
		msg := testMsg{"test", "hello", 23}
		ref, err := c.Publish(msg)
		r.NoError(err, "failed to call publish")
		r.NotNil(ref)

		wantRefs = append(wantRefs, ref.Ref())
	}

	seqv, err = srv.ReceiveLog.Seq().Value()
	r.NoError(err, "failed to get root log sequence")
	r.EqualValues(9, seqv)

	args := message.CreateHistArgs{
		ID:     testKP.Id,
		AsJSON: true,
	}
	src, err := c.CreateHistoryStream(args)
	r.NoError(err)

	ctx := context.TODO()
	for i := 0; i < 10; i++ {
		// ctx, _ := context.WithTimeout(ctx, 5*time.Second)
		ok := src.Next(ctx)
		r.True(ok, "expected more results")

		var v map[string]interface{}
		err = src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&v)
		})

		r.NoError(err, "failed JSON unmarshal message:%d", i)
		// a.Equal(wantRefs[i], msg.Key().Ref())
	}

	ok := src.Next(ctx)
	a.False(ok, "expected no more results")
	r.NoError(src.Err())

	// now with key-value wrapping
	args.Keys = true
	src, err = c.CreateHistoryStream(args)
	r.NoError(err)

	for i := 0; i < 10; i++ {
		// ctx, _ := context.WithTimeout(ctx, 5*time.Second)
		ok := src.Next(ctx)
		r.True(ok, "expected more results")

		var msg refs.KeyValueRaw
		err = src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&msg)
		})

		var v testMsg
		spew.Dump(msg.Value.Content)
		err = json.Unmarshal(msg.Value.Content, &v)
		r.NoError(err, "failed JSON unmarshal message:%d", i)
		// a.Equal(wantRefs[i], msg.Key().Ref())
	}

	ok = src.Next(ctx)
	a.False(ok, "expected no more results")
	r.NoError(src.Err())

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
