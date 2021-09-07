// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestReadStreamAsInterfaceMessage(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)

	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"))
	r.NoError(err, "sbot srv init failed")

	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(context.TODO())
		if err != nil {
			srvErrc <- fmt.Errorf("ali serve exited: %w", err)
		}
		close(srvErrc)
	}()

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()
	r.NotNil(srvAddr, "listener not ready")

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	// no messages yet
	r.Equal(margaret.SeqEmpty, srv.ReceiveLog.Seq())

	var wantRefs []string
	for i := 0; i < 10; i++ {
		msg := testMsg{"test", "hello", 23}
		ref, err := c.Publish(msg)
		r.NoError(err, "failed to call publish")
		r.NotNil(ref)

		// get stored message from the log
		wantSeq := int64(i)
		a.Equal(wantSeq, srv.ReceiveLog.Seq())
		msgv, err := srv.ReceiveLog.Get(wantSeq)
		r.NoError(err)
		newMsg, ok := msgv.(refs.Message)
		r.True(ok)
		r.Equal(newMsg.Key(), ref)
		wantRefs = append(wantRefs, ref.String())

		opts := message.CreateLogArgs{}
		opts.Keys = true
		opts.Limit = 1
		opts.Seq = int64(i)

		src, err := c.CreateLogStream(opts)
		r.NoError(err)

		ctx := context.TODO()
		r.True(src.Next(ctx))
		var streamMsg refs.KeyValueRaw
		err = src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&streamMsg)
		})
		r.NoError(err)

		a.Equal(newMsg.Author().String(), streamMsg.Author().String())

		a.EqualValues(newMsg.Seq(), streamMsg.Seq())

		r.False(src.Next(ctx))
		r.NoError(src.Err())
	}

	opts := message.CreateLogArgs{}
	opts.Keys = true
	opts.Limit = 10

	src, err := c.CreateLogStream(opts)
	r.NoError(err)

	ctx := context.TODO()
	for i := 0; i < 10; i++ {

		if !src.Next(ctx) {
			break
		}

		var msg refs.KeyValueRaw
		err = src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&msg)
		})
		r.NoError(err)

		a.Equal(wantRefs[i], msg.Key().String())
	}

	r.False(src.Next(ctx))
	r.NoError(src.Err())

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
