package sbot

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/multilogs"
)

func TestFeedsOneByOne(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	a := assert.New(t)
	ctx, cancel := context.WithCancel(context.TODO())

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	aliLog, _ := logtest.KitLogger("ali", t)
	// aliLog := log.NewLogfmtLogger(os.Stderr)
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(aliLog),
		// WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
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

	bobLog, _ := logtest.KitLogger("bob", t)
	// bobLog := log.NewLogfmtLogger(os.Stderr)
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

	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	alisLog, err := uf.Get(ali.KeyPair.Id.StoredAddr())
	r.NoError(err)

	for i := 0; i < 10; i++ {
		t.Logf("runniung connect %d", i)
		err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
		r.NoError(err)
		time.Sleep(10 * time.Millisecond)
		bob.Network.GetConnTracker().CloseAll()

		_, err := ali.PublishLog.Append(map[string]interface{}{
			"test": i,
		})
		r.NoError(err)

		seqv, err := alisLog.Seq().Value()
		r.NoError(err)
		a.Equal(margaret.BaseSeq(i), seqv, "check run %d", i)
	}

	auf, ok := ali.GetMultiLog("userFeeds")
	r.True(ok)
	ali.FSCK(auf)
	bob.FSCK(uf)

	ali.Shutdown()
	bob.Shutdown()

	r.NoError(ali.Close())
	r.NoError(bob.Close())

	r.NoError(<-mergeErrorChans(aliErrc, bobErrc))
	cancel()
}

// utils
func mergeErrorChans(cs ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error, 1)

	output := func(c <-chan error) {
		for a := range c {
			out <- a
		}
		wg.Done()
	}

	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
