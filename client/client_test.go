package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/message/legacy"
	"go.cryptoscope.co/ssb/sbot"
)

func TestUnixSock(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	// srvLog := log.NewJSONLogger(os.Stderr)
	srvLog, _ := logtest.KitLogger("srv", t)
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
	r.NoError(<-srvErrc)
}

func TestWhoami(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	// srvLog := log.NewJSONLogger(os.Stderr)
	srvLog, _ := logtest.KitLogger("srv", t)
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

func TestPublish(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	// srvLog := log.NewJSONLogger(os.Stderr)
	srvLog, _ := logtest.KitLogger("srv", t)
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

	src, err := c.CreateLogStream(message.CreateHistArgs{Limit: 1})
	r.NoError(err)

	streamV, err := src.Next(context.TODO())
	r.NoError(err)
	streamMsg, ok := streamV.(legacy.KeyValueAsMap)
	r.True(ok, "acutal type: %T", streamV)
	a.Equal(newMsg.Author().Ref(), streamMsg.Value.Author.Ref())
	a.EqualValues(newMsg.Seq(), streamMsg.Value.Sequence)

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
	// srvLog := log.NewJSONLogger(os.Stderr)
	srvLog, _ := logtest.KitLogger("srv", t)
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

	src, err := c.Tangles(*rootRef, message.CreateHistArgs{Limit: 2})
	r.NoError(err)

	streamV, err := src.Next(context.TODO())
	r.NoError(err)
	streamMsg, ok := streamV.(legacy.KeyValueAsMap)
	r.True(ok)

	a.EqualValues(2, streamMsg.Value.Sequence)

	streamV, err = src.Next(context.TODO())
	r.NoError(err)
	streamMsg, ok = streamV.(legacy.KeyValueAsMap)
	r.True(ok)
	a.EqualValues(3, streamMsg.Value.Sequence)

	v, err := src.Next(context.TODO())
	a.Nil(v)
	a.Equal(luigi.EOS{}, errors.Cause(err))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
