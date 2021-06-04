package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v3"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"
)

// todo: save the current state in the multilog
func makeSinkIndex(dbPath string, mlog multilog.MultiLog, fn multilog.Func) (librarian.SinkIndex, error) {
	statePath := filepath.Join(dbPath, "..", "state.json")
	mode := os.O_RDWR | os.O_EXCL
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		mode |= os.O_CREATE
	}
	idxStateFile, err := os.OpenFile(statePath, mode, 0700)
	if err != nil {
		return nil, fmt.Errorf("error opening state file: %w", err)
	}

	return multilog.NewSink(idxStateFile, mlog, fn), nil
}

const PrefixMultiLog = "sublogs"

func OpenBadgerDB(path string) (*badger.DB, error) {
	opts := badgerOpts(path)
	return badger.Open(opts)
}

/*
func OpenMultiLog(r Interface, name string, f multilog.Func) (multilog.MultiLog, librarian.SinkIndex, error) {
	return nil, nil, fmt.Errorf("TODO: deprecate me")
	dbPath := r.GetPath(PrefixMultiLog, name, "roaring-mkv")
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, fmt.Errorf("mkdir error for %q: %w", dbPath, err)
	}

	mkvPath := filepath.Join(dbPath, "db")
	mlog, err := multimkv.NewMultiLog(mkvPath)
	if err != nil {
		// yuk..
		if !isLockFileExistsErr(err) {
			// delete it if we cant recover it
			os.RemoveAll(dbPath)
			return nil, nil, fmt.Errorf("not a lockfile problem - deleting index: %w", err)
		}
		if err := cleanupLockFiles(dbPath); err != nil {
			return nil, nil, fmt.Errorf("failed to recover lockfiles: %w", err)

		}
		mlog, err = multimkv.NewMultiLog(mkvPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open roaring db: %w", err)
		}
	}

	snk, err := makeSinkIndex(dbPath, mlog, f)
	if err != nil {
		return nil, nil, fmt.Errorf("mlog/fs: failed to create sink: %w", err)
	}

	return mlog, snk, nil
}
*/
