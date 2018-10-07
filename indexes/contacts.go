package indexes

import (
	"context"

	"github.com/dgraph-io/badger"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/repo"
)

func OpenContacts(log kitlog.Logger, r repo.Interface) (graph.Builder, func(context.Context, margaret.Log) error, error) {
	f := func(db *badger.DB) librarian.SinkIndex {
		return graph.NewBuilder(kitlog.With(log, "module", "graph"), db)
	}

	_, sinkIdx, serve, err := repo.OpenBadgerIndex(r, "contacts", f)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting contacts index")
	}

	bldr := sinkIdx.(graph.Builder)

	return bldr, serve, nil
}
