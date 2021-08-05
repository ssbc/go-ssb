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

	uf, ok := srv.GetMultiLog("userFeeds")
	r.True(ok)

	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(context.TODO())
		if err != nil {
			srvErrc <- fmt.Errorf("ali serve exited: %w", err)
		}
		close(srvErrc)
	}()

	var testKeyPairs = make(map[string]int, 10)
	var i int
	for i = 0; i < 10; i++ {
		var algo = refs.RefAlgoFeedSSB1
		if i%2 == 0 {
			algo = refs.RefAlgoFeedGabby
		}
		kp, err := ssb.NewKeyPair(nil, algo)
		r.NoError(err)

		publish, err := message.OpenPublishLog(srv.ReceiveLog, uf, kp)
		r.NoError(err)

		testKeyPairs[kp.ID().Ref()] = i
		for n := i; n > 0; n-- {
			ref, err := publish.Publish(struct {
				Type  string `json:"type"`
				Test  bool
				N     int
				Hello string
			}{"test", true, n, kp.ID().Ref()})
			r.NoError(err)
			t.Log(ref.Ref())
		}
	}

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	src, err := c.ReplicateUpTo()
	r.NoError(err)

	ctx := context.TODO()
	for i = 0; true; i++ {
		if !src.Next(ctx) {
			break
		}

		var upToResp ssb.ReplicateUpToResponse
		err = src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&upToResp)
		})
		r.NoError(err)

		ref := upToResp.ID.Ref()
		i, has := testKeyPairs[ref]
		a.True(has, "upTo not in created set:%s", ref)
		a.EqualValues(i, upToResp.Sequence)
	}
	r.Equal(i, 9)

	r.False(src.Next(ctx))
	r.NoError(src.Err())

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
