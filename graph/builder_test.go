// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package graph

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"

	"go.cryptoscope.co/margaret/multilog"
	multibadger "go.cryptoscope.co/margaret/multilog/roaring/badger"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
)

func openUserMultilogs(t *testing.T) (multilog.MultiLog, multilog.Sink) {
	r := require.New(t)

	tRepoPath := filepath.Join("testrun", t.Name())

	db, err := repo.OpenBadgerDB(filepath.Join(tRepoPath, "badgerdb"))
	r.NoError(err)
	// seqSetterIdx := libbadger.NewIndex(db, 0)

	uf, err := multibadger.NewShared(db, []byte("userFeeds"))
	r.NoError(err)

	idxStatePath := filepath.Join(tRepoPath, "idx-state")
	idxStateFile, err := os.Create(idxStatePath)
	r.NoError(err)

	serveUF := multilog.NewSink(idxStateFile, uf, multilogs.UserFeedsUpdate)
	t.Cleanup(func() {
		if err := idxStateFile.Close(); err != nil {
			fmt.Println("failed to close idxStateFile:", err)
		}

		if err := db.Close(); err != nil {
			fmt.Println("failed to close badger:", err)
		}

		if err := serveUF.Close(); err != nil {
			fmt.Println("failed to close index serving:", err)
		}
	})
	return uf, serveUF
}

func makeBadger(t *testing.T) testStore {
	r := require.New(t)
	info := testutils.NewRelativeTimeLogger(nil)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)
	os.MkdirAll(tRepoPath, 0700)

	ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)

	// TODO: try this
	// tRootLog := mem.New()

	tRepo := repo.New(tRepoPath)
	tRootLog, err := repo.OpenLog(tRepo)
	r.NoError(err)

	uf, serveUF := openUserMultilogs(t)
	ufErrc := serveLog(ctx, "user feeds", tRootLog, serveUF, true)

	var builder *BadgerBuilder

	var tc testStore

	pth := tRepo.GetPath("contacts", "db")
	err = os.MkdirAll(pth, 0700)
	r.NoError(err, "error making index directory")

	badgerOpts := badger.DefaultOptions(pth).WithLoggingLevel(badger.ERROR)
	badgerDB, err := badger.Open(badgerOpts)
	r.NoError(err, "db/idx: badger failed to open")

	builder = NewBuilder(info, badgerDB, nil)

	idxSetter, idxContactsSink := builder.OpenContactsIndex()
	cErrc := serveLog(ctx, "badgerContacts", tRootLog, idxContactsSink, true)

	_, idxMetafeedsSink := builder.OpenMetafeedsIndex()
	mfErrc := serveLog(ctx, "badgerMetafeeds", tRootLog, idxMetafeedsSink, true)

	_, announcementSink := builder.OpenAnnouncementIndex()
	mfAnnounceErrc := serveLog(ctx, "badgerMetafeedAnnounce", tRootLog, announcementSink, true)

	tc.root = tRootLog
	tc.gbuilder = builder
	tc.userLogs = uf

	t.Cleanup(func() {
		r.NoError(uf.Close())

		r.NoError(idxContactsSink.Close())
		r.NoError(idxMetafeedsSink.Close())
		r.NoError(idxSetter.Close())

		r.NoError(badgerDB.Close())

		if t.Failed() {
			testutils.StreamLog(t, tRootLog)
		}
		r.NoError(tRootLog.Close())
		cancel()

		for err := range mergedErrors(ufErrc, cErrc, mfErrc, mfAnnounceErrc) {
			r.NoError(err, "from chan")
		}
		t.Log("closed scenary")
	})
	return tc
}

func TestBadger(t *testing.T) {
	tc := makeBadger(t)
	t.Run("scene1", tc.theScenario)
}

func makeTypedLog(t *testing.T) testStore {
	r := require.New(t)
	// info := testutils.NewRelativeTimeLogger(nil)

	tRepoPath, err := ioutil.TempDir("", "test_mlog")
	r.NoError(err)

	ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)

	tRepo := repo.New(tRepoPath)
	tRootLog, err := repo.OpenLog(tRepo)
	r.NoError(err)

	uf, serveUF := openUserMultilogs(t)
	ufErrc := serveLog(ctx, "user feeds", tRootLog, serveUF, true)

	var tc testStore
	tc.root = tRootLog
	tc.userLogs = uf

	t.Cleanup(func() {
		r.NoError(uf.Close())
		r.NoError(serveUF.Close())
		// r.NoError(mt.Close())

		cancel()

		for err := range mergedErrors(ufErrc) {
			r.NoError(err, "from chan")
		}
		t.Log("closed scenary")
	})

	panic("TODO: plugin refactor")
	/*
		mt, serveMT, err := repo.OpenMultiLog(tRepo, "byType", bytype.IndexUpdate)
		r.NoError(err, "sbot: failed to open message type sublogs")
		mtErrc := serveLog(ctx, "type logs", tRootLog, serveMT, true)

		contactLog, err := mt.Get(librarian.Addr("contact"))
		r.NoError(err, "sbot: failed to open message contact sublog")

		directedContactLog := mutil.Indirect(tRootLog, contactLog)
		tc.gbuilder, err = NewLogBuilder(info, directedContactLog)
		r.NoError(err, "sbot: NewLogBuilder failed")
	*/

	return tc
}

// TODO: logbuilder needs more love
func XTestTypedLog(t *testing.T) {
	tc := makeTypedLog(t)
	t.Run("scene1", tc.theScenario)
}

type testStore struct {
	root     margaret.Log
	userLogs multilog.MultiLog

	gbuilder Builder
}

func (tc testStore) newPublisher(t *testing.T) *publisher {
	return newPublisher(t, tc.root, tc.userLogs)
}

func (tc testStore) theScenario(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// some new people
	myself := tc.newPublisher(t)

	alice := tc.newPublisher(t)
	bob := tc.newPublisher(t)
	claire := tc.newPublisher(t)
	debby := tc.newPublisher(t)

	g, err := tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(0, g.NodeCount())

	auth := tc.gbuilder.Authorizer(myself.key.ID(), 0)

	// > create contacts
	myself.follow(alice.key.ID())
	myself.block(bob.key.ID())

	time.Sleep(time.Second / 10)

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	if !a.Equal(3, g.NodeCount()) {
		return
	}

	// not followed
	err = auth.Authorize(claire.key.ID())
	r.NotNil(err, "unknown ID")
	hopsErr, ok := err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.True(hopsErr.Dist < 0)

	// following
	err = auth.Authorize(alice.key.ID())
	r.Nil(err)

	// blocked
	err = auth.Authorize(bob.key.ID())
	r.NotNil(err, "no error for blocked peer")
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.True(hopsErr.Dist < 0)

	// alice follows claire
	alice.follow(claire.key.ID())

	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
		// TODO: expose index flushing
	}

	time.Sleep(time.Second / 10)

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(4, g.NodeCount())
	// r.NoError(g.RenderSVG())

	// now allowed. zero hops and not friends
	err = auth.Authorize(claire.key.ID())
	r.NotNil(err, "authorized wrong person (claire)")
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(1, hopsErr.Dist)
	r.Equal(0, hopsErr.Max)

	// alice follows me
	alice.follow(myself.key.ID())

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(4, g.NodeCount()) // same nodes more edges
	// r.NoError(g.RenderSVG())

	// now allowed. friends with alice but still 0 hops
	err = auth.Authorize(claire.key.ID())
	r.NotNil(err)
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(1, hopsErr.Dist)
	r.Equal(0, hopsErr.Max)

	// works for 1 hop
	h1 := tc.gbuilder.Authorizer(myself.key.ID(), 1)
	err = h1.Authorize(claire.key.ID())
	r.NoError(err)

	// claire follows debby
	claire.follow(debby.key.ID())

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(5, g.NodeCount()) // same nodes more edges
	// r.NoError(g.RenderSVG())

	err = h1.Authorize(debby.key.ID())
	r.NotNil(err)
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(2, hopsErr.Dist)
	r.Equal(1, hopsErr.Max)

	h2 := tc.gbuilder.Authorizer(myself.key.ID(), 2)
	err = h2.Authorize(debby.key.ID())
	r.Nil(err)
}

func serveLog(ctx context.Context, name string, l margaret.Log, snk librarian.SinkIndex, live bool) <-chan error {
	errc := make(chan error)
	go func() {
		defer close(errc)

		src, err := l.Query(snk.QuerySpec(), margaret.Live(live))
		if err != nil {
			log.Println("got err for", name, err)
			errc <- fmt.Errorf("%s query failed: %w", name, err)
			return
		}

		err = luigi.Pump(ctx, snk, src)
		if err != nil && !errors.Is(err, ssb.ErrShuttingDown) {
			log.Println("got err for", name, err)
			errc <- fmt.Errorf("%s serve exited: %w", name, err)
		}
	}()
	return errc
}

func mergedErrors(cs ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error)

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
