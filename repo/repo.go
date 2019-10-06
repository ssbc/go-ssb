// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/multilog"
	multibadger "go.cryptoscope.co/margaret/multilog/badger"
	"go.cryptoscope.co/margaret/multilog/roaringfiles"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
)

var _ Interface = repo{}

// New creates a new repository value, it opens the keypair and database from basePath if it is already existing
func New(basePath string) Interface {
	return repo{basePath: basePath}
}

type repo struct {
	basePath string
}

func (r repo) GetPath(rel ...string) string {
	return filepath.Join(append([]string{r.basePath}, rel...)...)
}

const PrefixMultiLog = "sublogs"

type ServeFunc func(context.Context, margaret.Log, bool) error

// OpenBadgerMultiLog uses the repo to determine the paths where to finds the multilog with given name and opens it.
//
// Exposes the badger db for 100% hackability. This will go away in future versions!
// badger + librarian as index
func OpenBadgerMultiLog(r Interface, name string, f multilog.Func) (multilog.MultiLog, ServeFunc, error) {

	dbPath := r.GetPath(PrefixMultiLog, name, "db")
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "mkdir error for %q", dbPath)
	}

	db, err := badger.Open(badgerOpts(dbPath))
	if err != nil {
		return nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	mlog := multibadger.New(db, msgpack.New(margaret.BaseSeq(0)))

	statePath := r.GetPath(PrefixMultiLog, name, "state.json")
	mode := os.O_RDWR | os.O_EXCL
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		mode |= os.O_CREATE
	}
	idxStateFile, err := os.OpenFile(statePath, mode, 0700)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error opening state file")
	}

	mlogSink := multilog.NewSink(idxStateFile, mlog, f)

	serve := func(ctx context.Context, rootLog margaret.Log, live bool) error {
		if rootLog == nil {
			return errors.Errorf("repo/multilog: %s was passed a nil root log", name)
		}

		src, err := rootLog.Query(margaret.Live(live), margaret.SeqWrap(true), mlogSink.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying rootLog for mlog")
		}

		err = luigi.Pump(ctx, mlogSink, src)
		if err == ssb.ErrShuttingDown {
			return nil
		}

		return errors.Wrap(err, "error reading query for mlog")
	}

	return mlog, serve, nil
}

func OpenMultiLog(r Interface, name string, f multilog.Func) (multilog.MultiLog, ServeFunc, error) {

	dbPath := r.GetPath(PrefixMultiLog, name, "roaring")
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "mkdir error for %q", dbPath)
	}

	mlog, err := roaringfiles.NewMKV(filepath.Join(dbPath, "mkv"))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to open roaring db")
	}

	if err := mlog.CompressAll(); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to compress db")
	}

	// todo: save the current state in the multilog
	statePath := r.GetPath(PrefixMultiLog, name, "state_mkv.json")
	mode := os.O_RDWR | os.O_EXCL
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		mode |= os.O_CREATE
	}
	idxStateFile, err := os.OpenFile(statePath, mode, 0700)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error opening state file")
	}

	mlogSink := multilog.NewSink(idxStateFile, mlog, f)

	serve := func(ctx context.Context, rootLog margaret.Log, live bool) error {
		if rootLog == nil {
			return errors.Errorf("repo/multilog: %s was passed a nil root log", name)
		}

		src, err := rootLog.Query(margaret.Live(live), margaret.SeqWrap(true), mlogSink.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying rootLog for mlog")
		}

		err = luigi.Pump(ctx, mlogSink, src)
		if err == ssb.ErrShuttingDown {
			return nil
		}

		return errors.Wrap(err, "error reading query for mlog")
	}

	return mlog, serve, nil
}

const PrefixIndex = "indexes"

func OpenIndex(r Interface, name string, f func(librarian.Index) librarian.SinkIndex) (librarian.Index, *badger.DB, ServeFunc, error) {
	pth := r.GetPath(PrefixIndex, name, "db")
	err := os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error making index directory")
	}

	db, err := badger.Open(badgerOpts(pth))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	idx := libbadger.NewIndex(db, 0)
	sinkidx := f(idx)

	serve := func(ctx context.Context, rootLog margaret.Log, live bool) error {
		src, err := rootLog.Query(margaret.Live(live), margaret.SeqWrap(true), sinkidx.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying root log")
		}

		err = luigi.Pump(ctx, sinkidx, src)
		if err == ssb.ErrShuttingDown {
			return nil
		}

		return errors.Wrap(err, "contacts index pump failed")
	}

	return idx, db, serve, nil
}

func OpenBadgerIndex(r Interface, name string, f func(*badger.DB) librarian.SinkIndex) (*badger.DB, librarian.SinkIndex, ServeFunc, error) {
	pth := r.GetPath(PrefixIndex, name, "db")
	err := os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error making index directory")
	}

	db, err := badger.Open(badgerOpts(pth))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	sinkidx := f(db)

	serve := func(ctx context.Context, rootLog margaret.Log, live bool) error {
		src, err := rootLog.Query(margaret.Live(live), margaret.SeqWrap(true), sinkidx.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying root log")
		}

		err = luigi.Pump(ctx, sinkidx, src)
		if err == ssb.ErrShuttingDown {
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
