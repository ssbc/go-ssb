// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2/debug"
	"go.cryptoscope.co/ssb"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/internal/broadcasts"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/testutils"
)

const blobSize = 1024 * 512

const testDelay = 7 * time.Second

func TestBlobsPair(t *testing.T) {
	defer leakcheck.Check(t)
	r := require.New(t)
	if testing.Short() {
		return
	}
	ctx, cancel := context.WithCancel(context.TODO())
	botgroup, ctx := errgroup.WithContext(ctx)

	info := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, info)

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	aliLog := log.With(info, "peer", "ali")
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(aliLog),
		WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
			return debug.WrapDump(filepath.Join("testrun", t.Name(), "muxdump"), conn)
		}),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(ali))

	bobLog := log.With(info, "peer", "bob")
	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(bobLog),
		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(bob))

	ali.Replicate(bob.KeyPair.ID())
	bob.Replicate(ali.KeyPair.ID())

	sess := &session{
		ctx:   ctx,
		alice: ali,
		bob:   bob,
		redial: func(t *testing.T) {
			t.Log("noop")
		},
	}

	tests := []struct {
		name string
		tf   func(t *testing.T)
	}{
		{"simple", sess.simple},
		{"wantFirst", sess.wantFirst},
		{"eachOne", sess.eachOne},
		{"eachOneConnet", sess.eachOneConnet},
		{"eachOneBothWant", sess.eachOnBothWant},
	}

	// all on a single connection
	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)
	for _, tc := range tests {
		t.Run("noop/"+tc.name, tc.tf)
	}

	info.Log("phase1", "done")

	aliCT := ali.Network.GetConnTracker()
	bobCT := bob.Network.GetConnTracker()
	aliCT.CloseAll()
	bobCT.CloseAll()
	i := 0
	an := aliCT.Count()
	bn := bobCT.Count()
	for an != 0 || bn != 0 {
		time.Sleep(750 * time.Millisecond)
		info.Log("event", "waited after close", "i", i, "a", an, "b", bn)
		i++
		if i > 10 {
			t.Fatal("retried waiting for close")
		}
		an = aliCT.Count()
		bn = bobCT.Count()
	}

	/* TODO: this fails _sometimes_

	// disconnect and reconnect
	sess.redial = func(t *testing.T) {
		aliCT.CloseAll()
		bobCT.CloseAll()
		time.Sleep(1 * time.Second)
		assert.EqualValues(t, 0, aliCT.Count(), "a: not all closed")
		assert.EqualValues(t, 0, bobCT.Count(), "b: not all closed")
		err := bob.Network.Connect(ctx, ali.Network.GetListenAddr())
		r.NoError(err)
		time.Sleep(testDelay)
		assert.EqualValues(t, 1, aliCT.Count(), "a: want 1 conn")
		assert.EqualValues(t, 1, bobCT.Count(), "b: want 1 conn")
	}
	for _, tc := range tests {
		t.Run("dcFirst/"+tc.name, tc.tf)
	}

	info.Log("block2", "done")

	aliCT.CloseAll()
	bobCT.CloseAll()
	time.Sleep(testDelay)
	assert.EqualValues(t, 0, aliCT.Count(), "a: not all closed")
	assert.EqualValues(t, 0, bobCT.Count(), "b: not all closed")

	// just re-dial
	sess.redial = func(t *testing.T) {
		info.Log("redial", "b>a")
		err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
		r.NoError(err)
		i := 0
		for aliCT.Count() < 1 || bobCT.Count() < 1 {
			time.Sleep(750 * time.Millisecond)
			info.Log("debugwait", "waited after connect", "i", i, "a", aliCT.Count(), "b", bobCT.Count())
			i++
			if i > 10 {
				info.Log("fail", "waited for conns failed")
				t.Fatal("retried dialing")
			}
		}

	}
	for _, tc := range tests {
		t.Run("redial/"+tc.name, tc.tf)
	}
	info.Log("block3", "done")
	*/

	ali.Shutdown()
	bob.Shutdown()
	cancel()

	r.NoError(ali.Close())
	r.NoError(bob.Close())
	r.NoError(botgroup.Wait())
}

type session struct {
	ctx context.Context

	redial func(t *testing.T)

	alice, bob *Sbot
}

// idea was to replace this with the sleep inbetween want and get assert
// TODO: add this to want itself to avoid race conditions
func blockUntilBlob(bot *Sbot, want refs.BlobRef) {
	received := make(chan struct{})
	emitter := func(nf ssb.BlobStoreNotification) error {
		eq := nf.Ref.Equal(want)
		bot.info.Log("update", nf.String(), "wanted", eq)

		if eq {
			// close(received)
			received <- struct{}{}
			bot.info.Log("chan", "closed")
		} else {
			bot.info.Log("unwanted", nf.Ref.Sigil())
		}
		return nil
	}

	done := bot.BlobStore.Register(broadcasts.BlobStoreFuncEmitter(emitter))

	select {
	case <-time.After(5 * time.Second):
		level.Warn(bot.info).Log("blob timeout", want.Sigil())
	case <-received:
	}
	done()
}

func (s *session) simple(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	s.redial(t)

	// blob action
	randBuf := make([]byte, blobSize)
	rand.Read(randBuf)

	ref, err := s.bob.BlobStore.Put(bytes.NewReader(randBuf))
	r.NoError(err)
	t.Log("added", ref.Sigil())

	err = s.alice.WantManager.Want(ref)
	r.NoError(err)

	time.Sleep(testDelay)

	_, err = s.alice.BlobStore.Get(ref)
	a.NoError(err)
}

func (s *session) wantFirst(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// blob action
	randBuf := make([]byte, blobSize)
	rand.Read(randBuf)

	ref, err := s.bob.BlobStore.Put(bytes.NewReader(randBuf))
	r.NoError(err)
	t.Log("added", ref.Sigil())

	err = s.alice.WantManager.Want(ref)
	r.NoError(err)

	s.redial(t)

	time.Sleep(testDelay)

	_, err = s.alice.BlobStore.Get(ref)
	a.NoError(err)

}

func (s *session) eachOne(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// blob action
	randOne := make([]byte, blobSize)
	rand.Read(randOne)
	refOne, err := s.bob.BlobStore.Put(bytes.NewReader(randOne))
	r.NoError(err)
	t.Log("added1", refOne.Sigil())

	randTwo := make([]byte, blobSize)
	rand.Read(randTwo)
	refTwo, err := s.alice.BlobStore.Put(bytes.NewReader(randTwo))
	r.NoError(err)
	t.Log("added2", refTwo.Sigil())

	s.redial(t)

	err = s.alice.WantManager.Want(refOne)
	r.NoError(err)

	err = s.bob.WantManager.Want(refTwo)
	r.NoError(err)

	time.Sleep(testDelay)

	_, err = s.alice.BlobStore.Get(refOne)
	a.NoError(err)
	_, err = s.bob.BlobStore.Get(refTwo)
	a.NoError(err)
}

func (s *session) eachOneConnet(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// blob action
	randOne := make([]byte, blobSize)
	rand.Read(randOne)
	refOne, err := s.bob.BlobStore.Put(bytes.NewReader(randOne))
	r.NoError(err)
	t.Log("added1", refOne.Sigil())

	randTwo := make([]byte, blobSize)
	rand.Read(randTwo)
	refTwo, err := s.alice.BlobStore.Put(bytes.NewReader(randTwo))
	r.NoError(err)
	t.Log("added2", refTwo.Sigil())

	err = s.alice.WantManager.Want(refOne)
	r.NoError(err)

	s.redial(t)

	err = s.bob.WantManager.Want(refTwo)
	r.NoError(err)

	time.Sleep(testDelay)

	_, err = s.alice.BlobStore.Get(refOne)
	a.NoError(err)
	_, err = s.bob.BlobStore.Get(refTwo)
	a.NoError(err)
}

func (s *session) eachOnBothWant(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// blob action
	randOne := make([]byte, blobSize)
	rand.Read(randOne)
	refOne, err := s.bob.BlobStore.Put(bytes.NewReader(randOne))
	r.NoError(err)
	t.Log("added1", refOne.Sigil())

	randTwo := make([]byte, blobSize)
	rand.Read(randTwo)
	refTwo, err := s.alice.BlobStore.Put(bytes.NewReader(randTwo))
	r.NoError(err)
	t.Log("added2", refTwo.Sigil())

	err = s.alice.WantManager.Want(refOne)
	r.NoError(err)

	err = s.bob.WantManager.Want(refTwo)
	r.NoError(err)

	s.redial(t)

	time.Sleep(5 * time.Second)

	_, err = s.alice.BlobStore.Get(refOne)
	a.NoError(err)
	_, err = s.bob.BlobStore.Get(refTwo)
	a.NoError(err)
}

// check that we can get blobs from C to A through B
func TestBlobsWithHops(t *testing.T) {
	defer leakcheck.Check(t)
	r := require.New(t)
	a := assert.New(t)

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	ctx, cancel := context.WithCancel(context.TODO())
	botgroup, ctx := errgroup.WithContext(ctx)

	mainLog := testutils.NewRelativeTimeLogger(nil)
	bs := newBotServer(ctx, mainLog)

	// make three bots (ali, bob and cle)
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "peer", "ali")),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(ali))

	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "peer", "bob")),
		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		// enabling this makes the tests hang but it can be insightfull to see all muxrpc packages
		// WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	addr := netwrap.GetAddr(conn.RemoteAddr(), "shs-bs")
		// 	return debug.WrapConn(log.With(mainLog, "remote", addr.String()[1:5]), conn), nil
		// }),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(bob))

	cle, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "peer", "cle")),
		WithRepoPath(filepath.Join("testrun", t.Name(), "cle")),
		WithListenAddr(":0"),
		// LateOption(MountPlugin(&bytype.Plugin{}, plugins2.AuthMaster)),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(cle))

	// ali <> bob
	ali.Replicate(bob.KeyPair.ID())
	bob.Replicate(ali.KeyPair.ID())

	// bob <> cle
	bob.Replicate(cle.KeyPair.ID())
	cle.Replicate(bob.KeyPair.ID())

	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)
	err = bob.Network.Connect(ctx, cle.Network.GetListenAddr())
	r.NoError(err)

	time.Sleep(1 * time.Second)

	// blob action
	n := blobSize
	randBuf := make([]byte, n)
	rand.Read(randBuf)

	ref, err := cle.BlobStore.Put(bytes.NewReader(randBuf))
	r.NoError(err)

	err = ali.WantManager.Want(ref)
	r.NoError(err)

	for i := 0; ali.WantManager.Wants(ref); i++ {
		time.Sleep(1 * time.Second)
		if i > 15 {
			t.Error("want timeout")
			break
		}
	}

	_, err = ali.BlobStore.Get(ref)
	a.NoError(err)

	sz, err := ali.BlobStore.Size(ref)
	a.NoError(err)
	a.EqualValues(n, sz)

	cancel()
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
	r.NoError(botgroup.Wait())
}

// TODO: make extra test to make sure this doesn't turn into an echo chamber

func TestBlobsTooBig(t *testing.T) {
	defer leakcheck.Check(t)
	r := require.New(t)
	a := assert.New(t)
	ctx, cancel := context.WithCancel(context.TODO())

	// <testSetup>
	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	info := testutils.NewRelativeTimeLogger(nil)

	srvGroup, ctx := errgroup.WithContext(ctx)
	srvBot := func(bot *Sbot, name string) {
		srvGroup.Go(func() error {
			err := bot.Network.Serve(ctx)
			if err != nil && err != context.Canceled {
				return fmt.Errorf("bot %s serve exited: %w", name, err)
			}
			return nil
		})
	}

	aliLog := log.With(info, "peer", "ali")
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(aliLog),
		// WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapConn(log.With(aliLog, "who", "a"), conn), nil
		// }),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	srvBot(ali, "ali")

	bobLog := log.With(info, "peer", "bob")
	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(bobLog),
		// WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	return debug.WrapConn(bobLog, conn), nil
		// }),
		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	srvBot(bob, "bob")

	blobUpdate := func(logger log.Logger) broadcasts.BlobStoreFuncEmitter {
		return broadcasts.BlobStoreFuncEmitter(func(nf ssb.BlobStoreNotification) error {
			logger.Log("blob", nf.Op.String(), "ref", nf.Ref.Sigil(), "size", nf.Size)
			return nil
		})
	}
	bob.BlobStore.Register(blobUpdate(bobLog))
	ali.BlobStore.Register(blobUpdate(aliLog))

	ali.Replicate(bob.KeyPair.ID())
	bob.Replicate(ali.KeyPair.ID())

	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)

	// </testSetup>

	// Ali adds a very big blob
	zerof, err := os.Open("/dev/zero")
	r.NoError(err)
	defer zerof.Close()

	const smallEnough = blobstore.DefaultMaxSize
	okayRef, err := ali.BlobStore.Put(io.LimitReader(zerof, smallEnough))
	r.NoError(err)
	t.Log("added small", okayRef.Sigil())

	sz, err := ali.BlobStore.Size(okayRef)
	r.NoError(err)
	a.EqualValues(smallEnough, sz)

	const veryLarge = blobstore.DefaultMaxSize + 1
	largeRef, err := ali.BlobStore.Put(io.LimitReader(zerof, veryLarge))
	r.NoError(err)
	t.Log("added too big", largeRef.Sigil())

	sz, err = ali.BlobStore.Size(largeRef)
	r.NoError(err)
	a.EqualValues(veryLarge, sz)

	err = bob.WantManager.Want(largeRef)
	r.NoError(err)
	err = bob.WantManager.Want(okayRef)
	r.NoError(err)

	time.Sleep(3 * time.Second)

	_, err = bob.BlobStore.Get(okayRef)
	a.NoError(err)

	_, err = bob.BlobStore.Get(largeRef)
	r.Error(err)
	a.Equal(err, blobstore.ErrNoSuchBlob)

	cancel()
	ali.Shutdown()
	bob.Shutdown()
	if err := srvGroup.Wait(); err != nil {
		t.Log(err)
	}

	// cleanup
	r.NoError(ali.Close())
	r.NoError(bob.Close())
}
