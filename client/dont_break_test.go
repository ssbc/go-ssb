// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

// this is only a basic test.
// ideally we would want to feed invalid messages to the verification
// and check that it doesn't drop the connection just because an error on one stream
func TestAskForSomethingWeird(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		// sbot.DisableNetworkNode(), skips muxrpc handler
		sbot.WithListenAddr(":0"),
	)
	r.NoError(err, "sbot srv init failed")

	ctx := context.TODO()
	ctx, cancel := context.WithCancel(ctx)
	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(ctx)
		if err != nil && err != context.Canceled {
			srvErrc <- fmt.Errorf("ali serve exited: %w", err)
		}
		close(srvErrc)
	}()

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	ref, err := c.Whoami()
	r.NoError(err, "failed to call whoami")
	r.NotNil(ref)
	a.Equal(srv.KeyPair.ID().String(), ref.String())

	// make sure we can publish
	var msgs []refs.MessageRef
	const msgCount = 15
	for i := 0; i < msgCount; i++ {
		ref, err := c.Publish(struct {
			Type string `json:"type"`
			Test int
		}{"test", i})
		r.NoError(err)
		r.NotNil(ref)
		msgs = append(msgs, ref)
	}

	// and stream those messages back
	var o message.CreateHistArgs
	o.ID = srv.KeyPair.ID()
	o.Keys = true
	o.Limit = -1
	src, err := c.CreateHistoryStream(o)
	r.NoError(err)
	r.NotNil(src)

	i := 0
	for {
		if !src.Next(ctx) {
			t.Log("hist stream ended", i)
			break
		}

		if i == 5 { // why after 5? - iirc its just somehwere inbetween the open stream
			var o message.CreateHistArgs
			o.ID, err = refs.NewFeedRefFromBytes(bytes.Repeat([]byte("nope"), 8), "wrong")
			r.NoError(err)

			o.Keys = true

			// starting the call works (although our lib could check that the ref is wrong, too)
			src, err := c.CreateHistoryStream(o)
			a.NoError(err)
			a.NotNil(src)
			a.False(src.Next(ctx))
			ce := src.Err()
			callErr, ok := ce.(*muxrpc.CallError)
			r.True(ok, "not a call err: %T", ce)
			t.Log(callErr)
		}

		var msg refs.KeyValueRaw
		err = src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&msg)
		})
		r.NoError(err)

		r.True(msg.Key().Equal(msgs[i]), "wrong message %d", i)
		i++
	}
	r.NoError(src.Err())
	r.Equal(msgCount, i, "did not get all messages")

	a.NoError(c.Close())
	cancel()
	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
