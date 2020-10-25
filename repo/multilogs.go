package repo

import (
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/multilog"
	multibadger "go.cryptoscope.co/margaret/multilog/badger"
	"go.cryptoscope.co/margaret/multilog/roaring"
	multifs "go.cryptoscope.co/margaret/multilog/roaring/fs"
	multimkv "go.cryptoscope.co/margaret/multilog/roaring/mkv"
)

const PrefixMultiLog = "sublogs"

// OpenBadgerMultiLog uses the repo to determine the paths where to finds the multilog with given name and opens it.
//
// Exposes the badger db for 100% hackability. This will go away in future versions!
// badger + librarian as index
func OpenBadgerMultiLog(r Interface, name string, f multilog.Func) (multilog.MultiLog, librarian.SinkIndex, error) {

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

	return mlog, mlogSink, nil
}

func OpenFileSystemMultiLog(r Interface, name string, f multilog.Func) (*roaring.MultiLog, librarian.SinkIndex, error) {
	dbPath := r.GetPath(PrefixMultiLog, name, "fs-bitmaps")
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "mkdir error for %q", dbPath)
	}
	mlog, err := multifs.NewMultiLog(dbPath)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "open error for %q", dbPath)
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

	return mlog, mlogSink, nil

}

func OpenMultiLog(r Interface, name string, f multilog.Func) (multilog.MultiLog, librarian.SinkIndex, error) {

	dbPath := r.GetPath(PrefixMultiLog, name, "roaring")
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "mkdir error for %q", dbPath)
	}

	mkvPath := filepath.Join(dbPath, "mkv")
	mlog, err := multimkv.NewMultiLog(mkvPath)
	if err != nil {
		// yuk..
		if !isLockFileExistsErr(err) {
			// delete it if we cant recover it
			os.RemoveAll(dbPath)
			return nil, nil, errors.Wrapf(err, "not a lockfile problem - deleting index")
		}
		if err := cleanupLockFiles(dbPath); err != nil {
			return nil, nil, errors.Wrapf(err, "failed to recover lockfiles")

		}
		mlog, err = multimkv.NewMultiLog(mkvPath)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to open roaring db")
		}
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

	return mlog, mlogSink, nil
}
