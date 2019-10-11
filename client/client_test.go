// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.cryptoscope.co/muxrpc"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb/plugins2"

	"github.com/cryptix/go/logging/logtest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins2/tangles"
	"go.cryptoscope.co/ssb/sbot"
)

func TestUnixSock(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := newReltimeLogger()

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		// sbot.DisableNetworkNode(), skips muxrpc handler
		sbot.WithListenAddr(":0"),
		sbot.WithUNIXSocket(),
	)
	r.NoError(err, "sbot srv init failed")

	c, err := client.NewUnix(context.TODO(), filepath.Join(srvRepo, "socket"))
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	ref, err := c.Whoami()
	r.NoError(err, "failed to call whoami")
	r.NotNil(ref)
	a.Equal(srv.KeyPair.Id.Ref(), ref.Ref())

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
}

func TestWhoami(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := newReltimeLogger()

	srv, err := sbot.New(
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

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(context.TODO(), kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	ref, err := c.Whoami()
	r.NoError(err, "failed to call whoami")
	r.NotNil(ref)
	a.Equal(kp.Id.Ref(), ref.Ref())

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}

func TestLotsOfWhoami(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := newReltimeLogger()

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		// sbot.DisableNetworkNode(), skips muxrpc handler
		sbot.WithListenAddr(":0"),
		sbot.WithUNIXSocket(),
	)
	r.NoError(err, "sbot srv init failed")

	c, err := client.NewUnix(context.TODO(), filepath.Join(srvRepo, "socket"))
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	for i := 50; i > 0; i-- {
		ref, err := c.Whoami()
		r.NoError(err, "call %d errored", i)
		r.NotNil(ref)
		a.Equal(srv.KeyPair.Id.Ref(), ref.Ref(), "call %d has wrong result", i)
	}

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
}

func TestLotsOfStatus(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := newReltimeLogger()

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		// sbot.DisableNetworkNode(), skips muxrpc handler
		sbot.WithListenAddr(":0"),
		sbot.WithUNIXSocket(),
	)
	r.NoError(err, "sbot srv init failed")

	ctx, done := context.WithCancel(context.Background())

	g, ctx := errgroup.WithContext(ctx)

	n := 25
	for i := n; i > 0; i-- {
		fn := func() error {
			tick := time.NewTicker(250 * time.Millisecond)
			for range tick.C {
				select {
				case <-ctx.Done():
					return nil
				default:

				}
				c, err := client.NewUnix(ctx, filepath.Join(srvRepo, "socket"))
				if err != nil {
					return errors.Wrap(err, "failed to make client connection")
				}
				// end test boilerplate

				_, err = c.Async(ctx, map[string]interface{}{}, muxrpc.Method{"status"})
				if err != nil {
					if errors.Cause(err) == context.Canceled {
						return nil
					}
					return errors.Wrapf(err, "tick%p failed", tick)
				}
				if err := c.Close(); err != nil {
					return errors.Wrapf(err, "tick%p failed close", tick)
				}

			}
			return nil
		}
		g.Go(fn)
	}

	c, err := client.NewUnix(ctx, filepath.Join(srvRepo, "socket"))
	r.NoError(err, "failed to make client connection")

	for i := 25; i > 0; i-- {
		time.Sleep(500 * time.Millisecond)
		ref, err := c.Publish(struct{ Test int }{i})
		r.NoError(err, "publish %d errored", i)
		r.NotNil(ref)
	}
	time.Sleep(1 * time.Second)

	done()

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(g.Wait())
}

func TestPublish(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)

	srvLog := newReltimeLogger()

	srv, err := sbot.New(
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

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(context.TODO(), kp, srvAddr)
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
	msg := testMsg{"hello", 23}
	ref, err := c.Publish(msg)
	r.NoError(err, "failed to call publish")
	r.NotNil(ref)

	// get stored message from the log
	seqv, err = srv.RootLog.Seq().Value()
	r.NoError(err, "failed to get root log sequence")
	wantSeq := margaret.BaseSeq(0)
	a.Equal(wantSeq, seqv)
	msgv, err := srv.RootLog.Get(wantSeq)
	r.NoError(err)
	newMsg, ok := msgv.(ssb.Message)
	r.True(ok)
	r.Equal(newMsg.Key(), ref)

	opts := message.CreateLogArgs{}
	opts.Limit = 1
	opts.Keys = true
	opts.MarshalType = ssb.KeyValueRaw{}
	src, err := c.CreateLogStream(opts)
	r.NoError(err)

	streamV, err := src.Next(context.TODO())
	r.NoError(err)
	streamMsg, ok := streamV.(ssb.Message)
	r.True(ok, "acutal type: %T", streamV)
	a.Equal(newMsg.Author().Ref(), streamMsg.Author().Ref())
	a.EqualValues(newMsg.Seq(), streamMsg.Seq())

	v, err := src.Next(context.TODO())
	a.Nil(v)
	a.Equal(luigi.EOS{}, errors.Cause(err))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}

func TestTangles(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	// srvLog := newReltimeLogger()
	srvLog, _ := logtest.KitLogger("srv", t)
	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.MountPlugin(&tangles.Plugin{}, plugins2.AuthMaster)),
	)
	r.NoError(err, "sbot srv init failed")

	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(context.TODO())
		if err != nil {
			srvErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(srvErrc)
	}()

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(context.TODO(), kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	type testMsg struct {
		Foo  string
		Bar  int
		Root *ssb.MessageRef `json:"root,omitempty"`
	}
	msg := testMsg{"hello", 23, nil}
	rootRef, err := c.Publish(msg)
	r.NoError(err, "failed to call publish")
	r.NotNil(rootRef)

	rep1 := testMsg{"reply", 1, rootRef}
	rep1Ref, err := c.Publish(rep1)
	r.NoError(err, "failed to call publish")
	r.NotNil(rep1Ref)
	rep2 := testMsg{"reply", 2, rootRef}
	rep2Ref, err := c.Publish(rep2)
	r.NoError(err, "failed to call publish")
	r.NotNil(rep2Ref)

	opts := message.TanglesArgs{}
	opts.Root = *rootRef
	opts.Limit = 2
	opts.Keys = true
	opts.MarshalType = ssb.KeyValueRaw{}
	src, err := c.Tangles(opts)
	r.NoError(err)

	streamV, err := src.Next(context.TODO())
	r.NoError(err)
	streamMsg, ok := streamV.(ssb.Message)
	r.True(ok)

	a.EqualValues(2, streamMsg.Seq())

	streamV, err = src.Next(context.TODO())
	r.NoError(err)
	streamMsg, ok = streamV.(ssb.Message)
	r.True(ok)
	a.EqualValues(3, streamMsg.Seq())

	v, err := src.Next(context.TODO())
	a.Nil(v)
	a.Equal(luigi.EOS{}, errors.Cause(err))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
