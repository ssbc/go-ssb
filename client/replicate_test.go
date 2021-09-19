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

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestReplicateUpTo(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)

	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
	)
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

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	// create a bunch of test feeds and messages
	const n = 10
	type keyAndCount struct {
		keyPair ssb.KeyPair
		count   int
	}
	var testKeyPairs = make(map[string]keyAndCount, n)
	var i int
	for i = 0; i < n; i++ {
		var algo = refs.RefAlgoFeedSSB1
		if i%2 == 0 {
			algo = refs.RefAlgoFeedGabby
		}
		kp, err := ssb.NewKeyPair(nil, algo)
		r.NoError(err)

		publish, err := message.OpenPublishLog(srv.ReceiveLog, srv.Users, kp)
		r.NoError(err)

		testKeyPairs[kp.ID().String()] = keyAndCount{kp, i}
		for n := i; n > 0; n-- {
			ref, err := publish.Publish(struct {
				Type  string `json:"type"`
				Test  bool
				N     int
				Hello string
			}{"test", true, n, kp.ID().String()})
			r.NoError(err)
			t.Log(ref.Key().String())
		}
	}

	// create helper to create, drain the query and check the responses
	assertUpToResponse := func() int {
		src, err := c.ReplicateUpTo()
		r.NoError(err)

		ctx := context.TODO()
		var i int
		for i = 0; true; i++ {
			if !src.Next(ctx) {
				break
			}

			var upToResp ssb.ReplicateUpToResponse
			err = src.Reader(func(r io.Reader) error {
				return json.NewDecoder(r).Decode(&upToResp)
			})
			r.NoError(err)

			ref := upToResp.ID.String()
			// either it's one of the test keypairs
			kn, has := testKeyPairs[ref]
			if !has {
				// if not it's the server itself
				isSrv := upToResp.ID.Equal(srv.KeyPair.ID())
				a.True(isSrv, "unexpeted response feed: %+v", upToResp)
			}

			a.EqualValues(kn.count, upToResp.Sequence)
		}

		r.False(src.Next(ctx))
		r.NoError(src.Err())

		return i
	}

	r.Equal(1, assertUpToResponse(), "should only have self")

	// now actually replicate them
	for _, kp := range testKeyPairs {
		srv.Replicate(kp.keyPair.ID())
	}

	r.Equal(n+1, assertUpToResponse(), "should have all the feeds and self")

	// cleanup
	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
