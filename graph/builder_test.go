package graph

import (
	"context"
	"io/ioutil"
	"log"
	"sync"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
)

func TestBadger(t *testing.T) {
	r := require.New(t)
	info, _ := logtest.KitLogger(t.Name(), t)

	tRepoPath, err := ioutil.TempDir("", "badgerTest")
	r.NoError(err)

	ctx, cancel := context.WithCancel(context.TODO())

	tRepo := repo.New(tRepoPath)
	tRootLog, err := repo.OpenLog(tRepo)
	r.NoError(err)
	// TODO: try this
	// tRootLog := mem.New()
	uf, _, serveUF, err := multilogs.OpenUserFeeds(tRepo)
	r.NoError(err)
	ufErrc := serveLog(ctx, "user feeds", tRootLog, serveUF)

	var tc testCase
	_, sinkIdx, serve, err := repo.OpenBadgerIndex(tRepo, "contacts", func(db *badger.DB) librarian.SinkIndex {
		return NewBuilder(info, db)
	})
	r.NoError(err)
	cErrc := serveLog(ctx, "badgerContacts", tRootLog, serve)
	tc.root = tRootLog
	tc.gbuilder = sinkIdx.(Builder)
	tc.userLogs = uf

	t.Run("scene1", tc.theScenario)

	// cleanup
	r.NoError(uf.Close())
	r.NoError(sinkIdx.Close())
	cancel()

	for err := range mergedErrors(ufErrc, cErrc) {
		r.NoError(err, "from chan")
	}
}

func TestTypedLog(t *testing.T) {
	r := require.New(t)
	info, _ := logtest.KitLogger(t.Name(), t)

	tRepoPath, err := ioutil.TempDir("", "test_mlog")
	r.NoError(err)

	ctx, cancel := context.WithCancel(context.TODO())

	tRepo := repo.New(tRepoPath)
	tRootLog, err := repo.OpenLog(tRepo)
	r.NoError(err)

	uf, _, serveUF, err := multilogs.OpenUserFeeds(tRepo)
	r.NoError(err)
	ufErrc := serveLog(ctx, "user feeds", tRootLog, serveUF)

	var typeLogCase testCase
	typeLogCase.root = tRootLog
	typeLogCase.userLogs = uf

	mt, _, serveMT, err := multilogs.OpenMessageTypes(tRepo)
	r.NoError(err, "sbot: failed to open message type sublogs")
	mtErrc := serveLog(ctx, "type logs", tRootLog, serveMT)

	contactLog, err := mt.Get(librarian.Addr("contact"))
	r.NoError(err, "sbot: failed to open message contact sublog")

	directedContactLog := mutil.Indirect(tRootLog, contactLog)
	typeLogCase.gbuilder, err = NewLogBuilder(info, directedContactLog)
	r.NoError(err, "sbot: NewLogBuilder failed")

	t.Run("scene1", typeLogCase.theScenario)

	r.NoError(uf.Close())
	r.NoError(mt.Close())
	cancel()

	for err := range mergedErrors(ufErrc, mtErrc) {
		r.NoError(err, "from chan")
	}
}

type testCase struct {
	root     margaret.Log
	userLogs multilog.MultiLog

	gbuilder Builder
}

func (tc testCase) theScenario(t *testing.T) {
	r := require.New(t)
	// some new people

	myself := newPublisher(t, tc.root, tc.userLogs)

	alice := newPublisher(t, tc.root, tc.userLogs)
	bob := newPublisher(t, tc.root, tc.userLogs)
	claire := newPublisher(t, tc.root, tc.userLogs)
	debby := newPublisher(t, tc.root, tc.userLogs)

	g, err := tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(0, g.NodeCount())

	a := tc.gbuilder.Authorizer(myself.key.Id, 0)

	// > create contacts
	myself.follow(alice.key.Id)
	myself.block(bob.key.Id)

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(3, g.NodeCount())

	// not followed
	err = a.Authorize(claire.key.Id)
	r.NotNil(err, "unknown ID")
	hopsErr, ok := err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.True(hopsErr.Dist < 0)

	// following
	err = a.Authorize(alice.key.Id)
	r.Nil(err)

	// blocked
	err = a.Authorize(bob.key.Id)
	r.NotNil(err, "no error for blocked peer")
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.True(hopsErr.Dist < 0)

	// alice follows claire
	alice.follow(claire.key.Id)

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(4, g.NodeCount())
	// r.NoError(g.RenderSVG())

	// now allowed. zero hops and not friends
	err = a.Authorize(claire.key.Id)
	r.NotNil(err)
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(1, hopsErr.Dist)
	r.Equal(0, hopsErr.Max)

	// alice follows me
	alice.follow(myself.key.Id)

	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(4, g.NodeCount()) // same nodes more edges
	// r.NoError(g.RenderSVG())

	// now allowed. friends with alice but still 0 hops
	err = a.Authorize(claire.key.Id)
	r.NotNil(err)
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(1, hopsErr.Dist)
	r.Equal(0, hopsErr.Max)

	// works for 1 hop
	h1 := tc.gbuilder.Authorizer(myself.key.Id, 1)
	err = h1.Authorize(claire.key.Id)
	r.NoError(err)

	// claire follows debby
	claire.follow(debby.key.Id)
	g, err = tc.gbuilder.Build()
	r.NoError(err)
	r.Equal(5, g.NodeCount()) // same nodes more edges
	// r.NoError(g.RenderSVG())

	err = h1.Authorize(debby.key.Id)
	r.NotNil(err)
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(2, hopsErr.Dist)
	r.Equal(1, hopsErr.Max)

	h2 := tc.gbuilder.Authorizer(myself.key.Id, 2)
	err = h2.Authorize(debby.key.Id)
	r.Nil(err)
}

type margaretServe func(context.Context, margaret.Log) error

func serveLog(ctx context.Context, name string, l margaret.Log, f margaretServe) <-chan error {
	errc := make(chan error)
	go func() {
		err := f(ctx, l)
		if err != nil {
			log.Println("got err for", name, err)
			errc <- errors.Wrapf(err, "%s serve exited", name)
		}
		close(errc)
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
