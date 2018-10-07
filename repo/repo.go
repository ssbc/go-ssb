package repo

import (
	"context"
	"os"
	"path"

	"github.com/cryptix/go/logging"
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"

	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/multilog"
	multibadger "go.cryptoscope.co/margaret/multilog/badger"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
)

var _ Interface = repo{}

var check = logging.CheckFatal

// New creates a new repository value, it opens the keypair and database from basePath if it is already existing
func New(basePath string) Interface {
	return repo{basePath: basePath}
}

type repo struct {
	basePath string
}

func (r repo) GetPath(rel ...string) string {
	return path.Join(append([]string{r.basePath}, rel...)...)
}

// OpenMultiLog uses the repo to determine the paths where to finds the multilog with given name and opens it.
//
// Exposes the badger db for 100% hackability. This will go away in future versions!
func OpenMultiLog(r Interface, name string, f multilog.Func) (multilog.MultiLog, *badger.DB, func(context.Context, margaret.Log) error, error) {
	// badger + librarian as index
	opts := badger.DefaultOptions

	dbPath := r.GetPath("sublogs", name, "db")
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "mkdir error for %q", dbPath)
	}

	opts.Dir = dbPath
	opts.ValueDir = opts.Dir // we have small values in this one

	db, err := badger.Open(opts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	mlog := multibadger.New(db, msgpack.New(margaret.BaseSeq(0)))

	statePath := r.GetPath("sublogs", name, "state.json")
	idxStateFile, err := os.OpenFile(statePath, os.O_CREATE|os.O_RDWR, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error opening state file")
	}

	mlogSink := multilog.NewSink(idxStateFile, mlog, f)

	serve := func(ctx context.Context, rootLog margaret.Log) error {
		src, err := rootLog.Query(margaret.Live(true), margaret.SeqWrap(true), mlogSink.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying rootLog for mlog")
		}

		err = luigi.Pump(ctx, mlogSink, src)
		if err == context.Canceled {
			return nil
		}

		return errors.Wrap(err, "error reading query for mlog")
	}

	return mlog, db, serve, nil
}

func OpenIndex(r Interface, name string, f func(librarian.Index) librarian.SinkIndex) (librarian.Index, *badger.DB, func(context.Context, margaret.Log) error, error) {
	pth := r.GetPath("indexes", name, "db")
	err := os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error making index directory")
	}

	opts := badger.DefaultOptions
	opts.Dir = pth
	opts.ValueDir = opts.Dir

	db, err := badger.Open(opts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	idx := libbadger.NewIndex(db, 0)
	sinkidx := f(idx)

	serve := func(ctx context.Context, rootLog margaret.Log) error {
		src, err := rootLog.Query(margaret.Live(true), margaret.SeqWrap(true), sinkidx.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying root log")
		}

		err = luigi.Pump(ctx, sinkidx, src)
		if err == nil || err == context.Canceled {
			return nil
		}

		return errors.Wrap(err, "contacts index pump failed")
	}

	return idx, db, serve, nil
}

func OpenBadgerIndex(r Interface, name string, f func(*badger.DB) librarian.SinkIndex) (*badger.DB, librarian.SinkIndex, func(context.Context, margaret.Log) error, error) {
	pth := r.GetPath("indexes", name, "db")
	err := os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error making index directory")
	}

	opts := badger.DefaultOptions
	opts.Dir = pth
	opts.ValueDir = opts.Dir

	db, err := badger.Open(opts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	sinkidx := f(db)

	serve := func(ctx context.Context, rootLog margaret.Log) error {
		src, err := rootLog.Query(margaret.Live(true), margaret.SeqWrap(true), sinkidx.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying root log")
		}

		err = luigi.Pump(ctx, sinkidx, src)
		if err == nil || err == context.Canceled {
			return nil
		}

		return errors.Wrap(err, "contacts index pump failed")
	}

	return db, sinkidx, serve, nil
}

func OpenBlobStore(r Interface) (ssb.BlobStore, error) {
	bs, err := blobstore.New(r.GetPath("blobs"))
	return bs, errors.Wrap(err, "error opening blob store")
}
