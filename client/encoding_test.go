// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
)

func TestEncodeHistStreamAsJSON(t *testing.T) {
	defer leakcheck.Check(t)
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)

	srvLog := testutils.NewRelativeTimeLogger(nil)

	testKP, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	testKP.Id.Algo = ssb.RefAlgoFeedGabby

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

	c, err := client.NewTCP(context.TODO(), testKP, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	// no messages yet
	seqv, err := srv.RootLog.Seq().Value()
	r.NoError(err, "failed to get root log sequence")
	r.Equal(margaret.SeqEmpty, seqv)

	type testMsg struct {
		Foo string
		Bar int
	}
	var refs []string
	for i := 0; i < 10; i++ {

		msg := testMsg{"hello", 23}
		ref, err := c.Publish(msg)
		r.NoError(err, "failed to call publish")
		r.NotNil(ref)

		refs = append(refs, ref.Ref())
	}

	seqv, err = srv.RootLog.Seq().Value()
	r.NoError(err, "failed to get root log sequence")
	r.EqualValues(9, seqv)

	args := message.CreateHistArgs{
		ID:     testKP.Id,
		AsJSON: true,
	}
	args.MarshalType = json.RawMessage{}
	src, err := c.CreateHistoryStream(args)
	r.NoError(err)

	ctx := context.TODO()
	for i := 0; i < 10; i++ {
		// ctx, _ := context.WithTimeout(ctx, 5*time.Second)
		streamV, err := src.Next(ctx)
		r.NoError(err, "failed to next msg:%d", i)
		msg, ok := streamV.(json.RawMessage)
		r.True(ok, "acutal type: %T", streamV)

		var v map[string]interface{}
		err = json.Unmarshal(msg, &v)
		r.NoError(err, "failed JSON unmarshal message:%d", i)
		// a.Equal(refs[i], msg.Key().Ref())
	}

	v, err := src.Next(context.TODO())
	a.Nil(v)
	a.Equal(luigi.EOS{}, errors.Cause(err))

	// now with key-value wrapping
	args.MarshalType = &ssb.KeyValueRaw{}
	args.Keys = true
	src, err = c.CreateHistoryStream(args)
	r.NoError(err)

	ctx = context.TODO()
	for i := 0; i < 10; i++ {
		// ctx, _ := context.WithTimeout(ctx, 5*time.Second)
		streamV, err := src.Next(ctx)
		r.NoError(err, "failed to next msg:%d", i)
		msg, ok := streamV.(*ssb.KeyValueRaw)
		r.True(ok, "acutal type: %T", streamV)

		var v testMsg
		spew.Dump(msg.Value.Content)
		err = json.Unmarshal(msg.Value.Content, &v)
		r.NoError(err, "failed JSON unmarshal message:%d", i)
		// a.Equal(refs[i], msg.Key().Ref())
	}

	v, err = src.Next(context.TODO())
	a.Nil(v)
	a.Equal(luigi.EOS{}, errors.Cause(err))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
