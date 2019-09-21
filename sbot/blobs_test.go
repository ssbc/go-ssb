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
	"go.cryptoscope.co/ssb/multilogs"
)

const blobSize = 64 * 1024 * 250

func TestBlobsPair(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	ctx, cancel := context.WithCancel(context.TODO())

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	info := log.NewLogfmtLogger(os.Stderr)
	// timestamps!
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

	info = log.With(info, "ts", log.Valuer(diffTime))

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
		LateOption(MountMultiLog("byTypes", multilogs.OpenMessageTypes)))
	r.NoError(err)

	var aliErrc = make(chan error, 1)
	go func() {
		err := ali.Network.Serve(ctx)
		if err != nil {
			aliErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(aliErrc)
	}()

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
		LateOption(MountMultiLog("byTypes", multilogs.OpenMessageTypes)))
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
	time.Sleep(250 * time.Millisecond)
	r.True(g.Follows(bob.KeyPair.Id, ali.KeyPair.Id))

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
		t.Run(tc.name, tc.tf)
	}

	info.Log("block1", "done")

	aliCT := ali.Network.GetConnTracker()
	bobCT := bob.Network.GetConnTracker()
	aliCT.CloseAll()
	bobCT.CloseAll()
	i := 0
	for aliCT.Count() != 0 || bobCT.Count() != 0 {
		time.Sleep(750 * time.Millisecond)
		info.Log("XXXX", "waited after close", "i", i, "a", aliCT.Count(), "b", bobCT.Count())
		i++
		if i > 10 {
			t.Fatal("retried waiting for close")
		}
	}

	// disconnect and reconnect
	sess.redial = func(t *testing.T) {
		aliCT.CloseAll()
		bobCT.CloseAll()
		time.Sleep(1 * time.Second)
		assert.EqualValues(t, 0, aliCT.Count(), "a: not all closed")
		assert.EqualValues(t, 0, bobCT.Count(), "b: not all closed")
		err := bob.Network.Connect(ctx, ali.Network.GetListenAddr())
		r.NoError(err)
		time.Sleep(2 * time.Second)
		assert.EqualValues(t, 1, aliCT.Count(), "a: want 1 conn")
		assert.EqualValues(t, 1, bobCT.Count(), "b: want 1 conn")
	}
	for _, tc := range tests {
		t.Run("dcFirst/"+tc.name, tc.tf)
	}

	info.Log("block2", "done")

	aliCT.CloseAll()
	bobCT.CloseAll()
	time.Sleep(2 * time.Second)
	assert.EqualValues(t, 0, aliCT.Count(), "a: not all closed")
	assert.EqualValues(t, 0, bobCT.Count(), "b: not all closed")

	// just re-dial
	sess.redial = func(t *testing.T) {
		info.Log("redial", "b>a")
		err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
		r.NoError(err)
		i := 0
		for aliCT.Count() != 1 || bobCT.Count() != 1 {
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

	ali.Shutdown()
	bob.Shutdown()

	r.NoError(ali.Close())
	r.NoError(bob.Close())

	r.NoError(<-mergeErrorChans(aliErrc, bobErrc))
	cancel()
}

type session struct {
	ctx context.Context

	redial func(t *testing.T)

	alice, bob *Sbot
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
	t.Log("added", ref.Ref())

	err = s.alice.WantManager.Want(ref)
	r.NoError(err)

	time.Sleep(2 * time.Second)

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
	t.Log("added", ref.Ref())

	err = s.alice.WantManager.Want(ref)
	r.NoError(err)

	s.redial(t)

	time.Sleep(2 * time.Second)

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
	t.Log("added1", refOne.Ref())

	randTwo := make([]byte, blobSize)
	rand.Read(randTwo)
	refTwo, err := s.alice.BlobStore.Put(bytes.NewReader(randTwo))
	r.NoError(err)
	t.Log("added2", refTwo.Ref())

	s.redial(t)

	err = s.alice.WantManager.Want(refOne)
	r.NoError(err)

	err = s.bob.WantManager.Want(refTwo)
	r.NoError(err)

	time.Sleep(2 * time.Second)

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
	t.Log("added1", refOne.Ref())

	randTwo := make([]byte, blobSize)
	rand.Read(randTwo)
	refTwo, err := s.alice.BlobStore.Put(bytes.NewReader(randTwo))
	r.NoError(err)
	t.Log("added2", refTwo.Ref())

	err = s.alice.WantManager.Want(refOne)
	r.NoError(err)

	s.redial(t)

	err = s.bob.WantManager.Want(refTwo)
	r.NoError(err)

	time.Sleep(2 * time.Second)

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
	t.Log("added1", refOne.Ref())

	randTwo := make([]byte, blobSize)
	rand.Read(randTwo)
	refTwo, err := s.alice.BlobStore.Put(bytes.NewReader(randTwo))
	r.NoError(err)
	t.Log("added2", refTwo.Ref())

	err = s.alice.WantManager.Want(refOne)
	r.NoError(err)

	err = s.bob.WantManager.Want(refTwo)
	r.NoError(err)

	s.redial(t)

	time.Sleep(2 * time.Second)

	_, err = s.alice.BlobStore.Get(refOne)
	a.NoError(err)
	_, err = s.bob.BlobStore.Get(refTwo)
	a.NoError(err)
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
		WithListenAddr(":0"),
		LateOption(MountMultiLog("byTypes", multilogs.OpenMessageTypes)))
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
		// WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
		// 	addr := netwrap.GetAddr(conn.RemoteAddr(), "shs-bs")
		// 	return debug.WrapConn(log.With(mainLog, "remote", addr.String()[1:5]), conn), nil
		// }),
		WithListenAddr(":0"),
		LateOption(MountMultiLog("byTypes", multilogs.OpenMessageTypes)))
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
		WithListenAddr(":0"),
		LateOption(MountMultiLog("byTypes", multilogs.OpenMessageTypes)))
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

	time.Sleep(1 * time.Second)

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
