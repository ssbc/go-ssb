package sbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

func TestBlobsSimple(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	a := assert.New(t)
	ctx, cancel := context.WithCancel(context.TODO())

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	// aliLog, _ := logtest.KitLogger("ali", t)
	aliLog := log.NewLogfmtLogger(os.Stderr)
	aliLog = log.With(aliLog, "peer", "ali")
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(aliLog),
		// WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapConn(log.With(aliLog, "who", "a"), conn), nil
		// }),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"))
	r.NoError(err)

	var aliErrc = make(chan error, 1)
	go func() {
		err := ali.Network.Serve(ctx)
		if err != nil {
			aliErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(aliErrc)
	}()

	// bobLog, _ := logtest.KitLogger("bob", t)
	bobLog := log.NewLogfmtLogger(os.Stderr)
	bobLog = log.With(bobLog, "peer", "bob")
	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(bobLog),
		// WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapConn(bobLog, conn), nil
		// }),
		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		WithListenAddr(":0"))
	r.NoError(err)

	var bobErrc = make(chan error, 1)
	go func() {
		err := bob.Network.Serve(ctx)
		if err != nil {
			bobErrc <- errors.Wrap(err, "bob serve exited")
		}
		close(bobErrc)
	}()

	seq, err := ali.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   bob.KeyPair.Id,
	})
	r.NoError(err)
	r.Equal(margaret.BaseSeq(0), seq)

	seq, err = bob.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   ali.KeyPair.Id,
	})
	r.NoError(err)

	g, err := bob.GraphBuilder.Build()
	r.NoError(err)
	r.True(g.Follows(bob.KeyPair.Id, ali.KeyPair.Id))

	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)

	// blob action
	randBuf := make([]byte, 8*1024)
	rand.Read(randBuf)

	ref, err := bob.BlobStore.Put(bytes.NewReader(randBuf))
	r.NoError(err)

	err = ali.WantManager.Want(ref)
	r.NoError(err)

	time.Sleep(1 * time.Second)

	_, err = ali.BlobStore.Get(ref)
	a.NoError(err)

	ali.Shutdown()
	bob.Shutdown()

	r.NoError(ali.Close())
	r.NoError(bob.Close())

	r.NoError(<-mergeErrorChans(aliErrc, bobErrc))
	cancel()
}

// check that we can get blobs from C to A through B
func TestBlobsWithHops(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	a := assert.New(t)
	ctx, cancel := context.WithCancel(context.TODO())

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	var l sync.Mutex
	start := time.Now()
	diffTime := func() interface{} {
		l.Lock()
		defer l.Unlock()
		newStart := time.Now()
		since := newStart.Sub(start)
		// start = newStart
		return since
	}

	mainLog := log.NewLogfmtLogger(os.Stderr)
	mainLog = log.With(mainLog, "t", log.Valuer(diffTime))

	// make three bots (ali, bob and cle)
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "peer", "ali")),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"))
	r.NoError(err)
	var aliErrc = make(chan error, 1)
	go func() {
		err := ali.Network.Serve(ctx)
		if err != nil {
			aliErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(aliErrc)
	}()

	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "peer", "bob")),
		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		// enabling this makes the tests hang but it can be insightfull to see all muxrpc packages
		// WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	addr := netwrap.GetAddr(conn.RemoteAddr(), "shs-bs")
		// 	return debug.WrapConn(log.With(bobLog, "remote", addr.String()), conn), nil
		// }),
		WithListenAddr(":0"))
	r.NoError(err)
	var bobErrc = make(chan error, 1)
	go func() {
		err := bob.Network.Serve(ctx)
		if err != nil {
			bobErrc <- errors.Wrap(err, "bob serve exited")
		}
		close(bobErrc)
	}()

	cle, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "peer", "cle")),
		WithRepoPath(filepath.Join("testrun", t.Name(), "cle")),
		WithListenAddr(":0"))
	r.NoError(err)
	var cleErrc = make(chan error, 1)
	go func() {
		err := cle.Network.Serve(ctx)
		if err != nil {
			cleErrc <- errors.Wrap(err, "cle serve exited")
		}
		close(cleErrc)
	}()

	// ali <> bob
	_, err = ali.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   bob.KeyPair.Id,
	})
	r.NoError(err)
	_, err = bob.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   ali.KeyPair.Id,
	})
	r.NoError(err)
	// bob <> cle
	_, err = bob.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   cle.KeyPair.Id,
	})
	r.NoError(err)
	_, err = cle.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   bob.KeyPair.Id,
	})
	r.NoError(err)

	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)
	err = bob.Network.Connect(ctx, cle.Network.GetListenAddr())
	r.NoError(err)

	time.Sleep(1 * time.Second)

	// blob action
	// n:=8*1024
	n := 128
	randBuf := make([]byte, n)
	rand.Read(randBuf)

	ref, err := cle.BlobStore.Put(bytes.NewReader(randBuf))
	r.NoError(err)

	err = ali.WantManager.Want(ref)
	r.NoError(err)

	time.Sleep(10 * time.Second)

	_, err = ali.BlobStore.Get(ref)
	a.NoError(err)

	sz, err := ali.BlobStore.Size(ref)
	a.NoError(err)
	a.EqualValues(n, sz)

	ali.Shutdown()
	bob.Shutdown()
	cle.Shutdown()

	// TODO:
	// a.False(ali.WantManager.Wants(ref), "a still wants")
	// a.False(bob.WantManager.Wants(ref), "b still wants")
	// a.False(cle.WantManager.Wants(ref), "c still wants")

	r.NoError(ali.Close())
	r.NoError(bob.Close())
	r.NoError(cle.Close())

	r.NoError(<-mergeErrorChans(aliErrc, bobErrc, cleErrc))
	cancel()
}

// TODO: make extra test to make sure this doesn't turn into an echo chamber
