package graph

import (
	"context"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
)

func TestFollows(t *testing.T) {
	// > test boilerplate
	// TODO: abstract serving and error channel handling
	// Meta TODO: close handling and status of indexing
	r := require.New(t)
	info, _ := logtest.KitLogger(t.Name(), t)

	tRepoPath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	ctx, cancel := context.WithCancel(context.TODO())

	tRepo := repo.New(tRepoPath)
	tRootLog, err := repo.OpenLog(tRepo)
	r.NoError(err)

	uf, _, serveUF, err := multilogs.OpenUserFeeds(tRepo)
	r.NoError(err)
	ufErrc := serveLog(ctx, "user feeds", tRootLog, serveUF)

	_, sinkIdx, serve, err := repo.OpenBadgerIndex(tRepo, "contacts", func(db *badger.DB) librarian.SinkIndex {
		return NewBuilder(info, db)
	})
	r.NoError(err)
	cErrc := serveLog(ctx, "contacts", tRootLog, serve)
	bldr := sinkIdx.(Builder)
	// < test boilerplate

	// some new people
	myself, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	myPublish, err := multilogs.OpenPublishLog(tRootLog, uf, *myself)
	r.NoError(err)

	alice, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	alicePublish, err := multilogs.OpenPublishLog(tRootLog, uf, *alice)
	r.NoError(err)

	bob, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	claire, err := ssb.NewKeyPair(nil)
	r.NoError(err)

	g, err := bldr.Build()
	r.NoError(err)
	r.Equal(0, g.Nodes())

	a := bldr.Authorizer(myself.Id, 0)

	// > create contacts
	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":      "contact",
			"contact":   alice.Id.Ref(),
			"following": true,
		},
		map[string]interface{}{
			"type":     "contact",
			"contact":  bob.Id.Ref(),
			"blocking": true,
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := myPublish.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}
	// < create contacts

	g, err = bldr.Build()
	r.NoError(err)
	r.Equal(3, g.Nodes())

	// not followed
	err = a.Authorize(claire.Id)
	r.NotNil(err, "unknown ID")
	hopsErr, ok := err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.True(hopsErr.Dist < 0)

	// following
	err = a.Authorize(alice.Id)
	r.Nil(err)

	// blocked
	err = a.Authorize(bob.Id)
	r.NotNil(err, "no error for blocked peer")
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.True(hopsErr.Dist < 0)

	// alice follows claire
	alicePublish.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   claire.Id.Ref(),
		"following": true,
	})
	g, err = bldr.Build()
	r.NoError(err)
	r.Equal(4, g.Nodes())
	r.NoError(g.RenderSVG())

	// now allowed. zero hops and not friends
	err = a.Authorize(claire.Id)
	r.NotNil(err)
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(2, hopsErr.Dist)
	r.Equal(0, hopsErr.Max)

	// alice follows me
	alicePublish.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   myself.Id.Ref(),
		"following": true,
	})
	g, err = bldr.Build()
	r.NoError(err)
	r.Equal(4, g.Nodes()) // same nodes more edges
	r.NoError(g.RenderSVG())

	// now allowed. friends but still 0 hops
	err = a.Authorize(claire.Id)
	r.NotNil(err)
	hopsErr, ok = err.(*ssb.ErrOutOfReach)
	r.True(ok, "acutal err: %T\n%+v", err, err)
	r.Equal(2, hopsErr.Dist)
	r.Equal(0, hopsErr.Max)

	uf.Close()
	sinkIdx.Close()
	cancel()

	for err := range mergedErrors(ufErrc, cErrc) {
		r.NoError(err, "from chan")
	}
}

type margaretServe func(context.Context, margaret.Log) error

func serveLog(ctx context.Context, name string, l margaret.Log, f margaretServe) <-chan error {
	errc := make(chan error)
	go func() {
		err := f(ctx, l)
		if err != nil {
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
