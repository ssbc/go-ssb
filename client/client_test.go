// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestUnixSock(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		// sbot.DisableNetworkNode(), skips muxrpc handler
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
	)
	r.NoError(err, "sbot srv init failed")

	c, err := client.NewUnix(filepath.Join(srvRepo, "socket"))
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

	ctx := context.TODO()
	i := 0
	for src.Next(ctx) {

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
	srv.Shutdown()
	r.NoError(srv.Close())
}

func TestWhoami(t *testing.T) {
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

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	ref, err := c.Whoami()
	r.NoError(err, "failed to call whoami")
	r.NotNil(ref)
	a.Equal(kp.ID().String(), ref.String())

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}

func TestLotsOfWhoami(t *testing.T) {
	// defer leakcheck.Check(t)
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		// sbot.DisableNetworkNode(), skips muxrpc handler
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
	)
	r.NoError(err, "sbot srv init failed")

	c, err := client.NewUnix(filepath.Join(srvRepo, "socket"))
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	for i := 50; i > 0; i-- {
		ref, err := c.Whoami()
		r.NoError(err, "call %d errored", i)
		r.NotNil(ref)
		a.Equal(srv.KeyPair.ID().String(), ref.String(), "call %d has wrong result", i)
	}

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
}

func TestStatusCalls(t *testing.T) {
	// defer leakcheck.Check(t)

	if testutils.SkipOnCI(t) {
		return
	}

	mkTCP := func(t *testing.T, opts ...sbot.Option) (*sbot.Sbot, mkClient) {
		r := require.New(t)

		srvRepo := filepath.Join("testrun", t.Name(), "serv")
		os.RemoveAll(srvRepo)
		srvLog := testutils.NewRelativeTimeLogger(nil)

		defOpts := []sbot.Option{
			sbot.WithInfo(srvLog),
			sbot.WithRepoPath(srvRepo),
			sbot.WithListenAddr(":0"),
		}

		srv, err := sbot.New(append(defOpts, opts...)...)
		r.NoError(err, "sbot srv init failed")
		ctx, cancel := context.WithCancel(context.TODO())
		t.Cleanup(func() {
			cancel()
		})
		go func() {
			err := srv.Network.Serve(ctx)
			t.Log("tcp bot serve exited", err)
			if err != nil && !errors.Is(err, context.Canceled) {
				panic(err)
			}
		}()

		kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
		r.NoError(err, "failed to load servers keypair")
		srvAddr := srv.Network.GetListenAddr()
		return srv, func(ctx context.Context) (*client.Client, error) {
			c, err := client.NewTCP(kp, srvAddr, client.WithContext(ctx))
			if err != nil {
				return nil, fmt.Errorf("failed to make TCP client connection: %w", err)
			}
			return c, nil
		}
	}

	mkUnix := func(t *testing.T, opts ...sbot.Option) (*sbot.Sbot, mkClient) {
		r := require.New(t)

		srvRepo := filepath.Join("testrun", t.Name(), "serv")
		os.RemoveAll(srvRepo)
		srvLog := testutils.NewRelativeTimeLogger(nil)

		defOpts := []sbot.Option{
			sbot.WithInfo(srvLog),
			sbot.WithRepoPath(srvRepo),
			sbot.WithListenAddr(":0"),
			sbot.LateOption(sbot.WithUNIXSocket()),
		}

		srv, err := sbot.New(append(defOpts, opts...)...)
		r.NoError(err, "sbot srv init failed")

		return srv, func(ctx context.Context) (*client.Client, error) {
			c, err := client.NewUnix(filepath.Join(srvRepo, "socket"), client.WithContext(ctx))
			if err != nil {
				return nil, fmt.Errorf("failed to make unix client connection: %w", err)
			}
			return c, nil
		}
	}

	// TODO: cleanup _send after close_ error
	//t.Run("tcp", LotsOfStatusCalls(mkTCP))
	_ = mkTCP
	t.Run("unix", LotsOfStatusCalls(mkUnix))
}

type mkClient func(context.Context) (*client.Client, error)
type mkPair func(t *testing.T, opts ...sbot.Option) (*sbot.Sbot, mkClient)

func LotsOfStatusCalls(newPair mkPair) func(t *testing.T) {

	return func(t *testing.T) {
		r, a := require.New(t), assert.New(t)

		srv, mkClient := newPair(t,
			// this test needs multiple stable client connections
			// the default, LastWinsTracker disconnects the previous connection
			sbot.WithNetworkConnTracker(network.NewAcceptAllTracker()),
		)
		r.NotNil(srv, "no server from init func")

		ctx, done := context.WithCancel(context.Background())

		g, ctx := errgroup.WithContext(ctx)

		var statusCalls uint32

		n := 25 // spawn n clients
		for i := n; i > 0; i-- {
			fn := func() error {
				tick := time.NewTicker(250 * time.Millisecond)
				for {
					select {
					case <-ctx.Done():
						return nil
					case <-tick.C:
					}

					c, err := mkClient(ctx)
					if err != nil {
						if errors.Is(err, context.Canceled) {
							return nil
						}
						return fmt.Errorf("tick%p failed: %w", tick, err)
					}

					var status map[string]interface{}
					err = c.Async(ctx, &status, muxrpc.TypeJSON, muxrpc.Method{"status"})
					if err != nil {
						if errors.Is(err, context.Canceled) || muxrpc.IsSinkClosed(err) {
							return nil
						}
						return fmt.Errorf("tick%p failed: %w", tick, err)
					}
					if err := c.Close(); err != nil {
						return fmt.Errorf("tick%p failed close: %w", tick, err)
					}
					// fmt.Println(resp)
					atomic.AddUint32(&statusCalls, 1)
				}
			}
			g.Go(fn)
		}

		c, err := mkClient(ctx)
		r.NoError(err, "failed to make client connection")

		// check that we can read messages as we create them from the same connection
		var lopt message.CreateLogArgs
		lopt.Live = true
		lopt.Keys = true
		lopt.Limit = -1
		src, err := c.CreateLogStream(lopt)
		r.NoError(err)

		for i := n; i > 0; i-- {
			time.Sleep(500 * time.Millisecond)
			ref, err := c.Publish(struct {
				Type string `json:"type"`
				Test int
			}{"test", i})
			r.NoError(err, "publish %d errored", i)
			r.NotNil(ref)

			r.True(src.Next(ctx))
			var msg refs.KeyValueRaw
			err = src.Reader(func(r io.Reader) error {
				return json.NewDecoder(r).Decode(&msg)
			})
			r.NoError(err, "message live err %d errored", i)

			a.True(msg.Key().Equal(ref), "wrong message: %d - %s", i, ref.String())
		}
		time.Sleep(1 * time.Second)
		a.NoError(c.Close())

		done()
		r.NoError(g.Wait())

		a.GreaterOrEqual(statusCalls, uint32(1000), "expected more status calls")

		r.EqualValues(n-1, srv.ReceiveLog.Seq())

		srv.Shutdown()
		r.NoError(srv.Close())
	}
}

type testMsg struct {
	Type string `json:"type"`
	Foo  string
	Bar  int
}

func TestPublish(t *testing.T) {
	// defer leakcheck.Check(t)
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

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	// no messages yet
	r.Equal(margaret.SeqEmpty, srv.ReceiveLog.Seq())

	msg := testMsg{"test", "hello", 23}
	ref, err := c.Publish(msg)
	r.NoError(err, "failed to call publish")
	r.NotNil(ref)

	// get stored message from the log
	wantSeq := int64(0)
	a.Equal(wantSeq, srv.ReceiveLog.Seq())
	msgv, err := srv.ReceiveLog.Get(wantSeq)
	r.NoError(err)
	newMsg, ok := msgv.(refs.Message)
	r.True(ok)
	r.Equal(newMsg.Key(), ref)

	opts := message.CreateLogArgs{}
	opts.Limit = 1
	opts.Keys = true
	src, err := c.CreateLogStream(opts)
	r.NoError(err)

	r.True(src.Next(context.TODO()))
	var streamMsg refs.KeyValueRaw
	err = src.Reader(func(r io.Reader) error {
		return json.NewDecoder(r).Decode(&streamMsg)
	})
	r.NoError(err)

	a.Equal(newMsg.Author().String(), streamMsg.Author().String())
	a.EqualValues(newMsg.Seq(), streamMsg.Seq())

	r.False(src.Next(context.TODO()))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}

func TestTanglesThread(t *testing.T) {
	// defer leakcheck.Check(t)
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"),
		// sbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapDump(filepath.Join(srvRepo, "muxdump"), conn)
		// }),
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

	type testMsg struct {
		Type   string           `json:"type"`
		Text   string           `json:"text"`
		Root   *refs.MessageRef `json:"root,omitempty"`
		Branch refs.MessageRefs `json:"branch,omitempty"`
	}
	msg := testMsg{"test", "hello", nil, nil}
	rootRef, err := c.Publish(msg)
	r.NoError(err, "failed to call publish")
	r.NotNil(rootRef)

	rep1 := testMsg{"test", "reply1", &rootRef, refs.MessageRefs{rootRef}}
	rep1Ref, err := c.Publish(rep1)
	r.NoError(err, "failed to call publish")
	r.NotNil(rep1Ref)
	rep2 := testMsg{"test", "reply2", &rootRef, refs.MessageRefs{rep1Ref}}
	rep2Ref, err := c.Publish(rep2)
	r.NoError(err, "failed to call publish")
	r.NotNil(rep2Ref)

	opts := message.TanglesArgs{}
	opts.Root = rootRef
	opts.Limit = 3
	opts.Keys = true
	src, err := c.TanglesThread(opts)
	r.NoError(err)

	ctx := context.TODO()
	r.True(src.Next(ctx), "did not get the 1st message: %v", src.Err())
	var streamMsg refs.KeyValueRaw
	err = src.Reader(decodeMuxMsg(&streamMsg))
	r.NoError(err, "did not decode message 1: %v", src.Err())
	a.EqualValues(1, streamMsg.Seq(), "got message %s", string(streamMsg.ContentBytes()))

	r.True(src.Next(ctx), "did not get the 2nd message: %v", src.Err())
	err = src.Reader(decodeMuxMsg(&streamMsg))
	r.NoError(err, "did not decode message 2: %v", src.Err())
	a.EqualValues(2, streamMsg.Seq(), "got message %s", string(streamMsg.ContentBytes()))

	r.True(src.Next(ctx), "did not get the 3rd message: %v", src.Err())
	err = src.Reader(decodeMuxMsg(&streamMsg))
	r.NoError(err, "did not decode message 3: %v", src.Err())
	a.EqualValues(3, streamMsg.Seq(), "got message %s", string(streamMsg.ContentBytes()))

	r.False(src.Next(ctx))
	r.NoError(src.Err())

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}

func decodeMuxMsg(msg interface{}) func(r io.Reader) error {
	return func(r io.Reader) error {
		return json.NewDecoder(r).Decode(msg)
	}
}
