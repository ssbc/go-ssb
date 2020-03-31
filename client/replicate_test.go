// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
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
			srvErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(srvErrc)
	}()

	var testKeyPairs = make(map[string]int, 10)
	var i int
	for i = 0; i < 10; i++ {
		kp, err := ssb.NewKeyPair(nil)
		r.NoError(err)
		if i%2 == 0 {
			kp.Id.Algo = ssb.RefAlgoFeedGabby
		}

		publish, err := message.OpenPublishLog(srv.RootLog, uf, kp)
		r.NoError(err)

		testKeyPairs[kp.Id.Ref()] = i
		for n := i; n > 0; n-- {

			ref, err := publish.Publish(struct {
				Test  bool
				N     int
				Hello string
			}{true, n, kp.Id.Ref()})
			r.NoError(err)
			t.Log(ref.Ref())
		}
	}

	// c, err := client.NewUnix(context.TODO(), filepath.Join(srvRepo, "socket"))
	// r.NoError(err, "failed to make client connection")
	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	src, err := c.ReplicateUpTo()
	r.NoError(err)

	for i = 0; true; i++ {
		streamV, err := src.Next(context.TODO())
		if luigi.IsEOS(err) {
			break
		}
		r.NoError(err, "i:%d", i)

		upToResp, ok := streamV.(ssb.ReplicateUpToResponse)
		r.True(ok, "type: %T", streamV)

		ref := upToResp.ID.Ref()
		i, has := testKeyPairs[ref]
		a.True(has, "upTo not in created set:%s", ref)
		a.EqualValues(i, upToResp.Sequence)
	}
	r.Equal(i, 9)

	v, err := src.Next(context.TODO())
	a.Nil(v)
	a.True(luigi.IsEOS(err))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
