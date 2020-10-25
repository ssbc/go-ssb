package repo

import (
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libmkv "go.cryptoscope.co/librarian/mkv"
	"go.cryptoscope.co/margaret"
	"modernc.org/kv"
)

const PrefixIndex = "indexes"

func OpenIndex(r Interface, name string, f func(librarian.SeqSetterIndex) librarian.SinkIndex) (librarian.Index, librarian.SinkIndex, error) {
	pth := r.GetPath(PrefixIndex, name, "mkv")
	err := os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, nil, errors.Wrap(err, "openIndex: error making index directory")
	}

	db, err := OpenMKV(pth)
	if err != nil {
		return nil, nil, errors.Wrap(err, "openIndex: failed to open MKV database")
	}

	idx := libmkv.NewIndex(db, margaret.BaseSeq(0))
	return idx, f(idx), nil
}

func OpenMKV(pth string) (*kv.DB, error) {
	opts := &kv.Options{}
	os.MkdirAll(pth, 0700)
	dbname := filepath.Join(pth, "idx.mkv")
	var db *kv.DB
	_, err := os.Stat(dbname)
	if os.IsNotExist(err) {
		db, err = kv.Create(dbname, opts)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create mkv")
		}
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to stat path location")
	} else {
		db, err = kv.Open(dbname, opts)
		if err != nil {

			if !isLockFileExistsErr(err) {
				return nil, err
			}
			if err := cleanupLockFiles(pth); err != nil {
				return nil, errors.Wrapf(err, "failed to recover lockfiles")
			}
			db, err = kv.Open(dbname, opts)
			if err != nil {
				return nil, errors.Wrap(err, "failed to open mkv")
			}
		}
	}
	return db, nil
}

type LibrarianIndexCreater func(*badger.DB) (librarian.SeqSetterIndex, librarian.SinkIndex)

func OpenBadgerIndex(r Interface, name string, f LibrarianIndexCreater) (*badger.DB, librarian.SeqSetterIndex, librarian.SinkIndex, error) {
	pth := r.GetPath(PrefixIndex, name, "db")
	err := os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error making index directory")
	}

	db, err := badger.Open(badgerOpts(pth))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	idx, sinkidx := f(db)

	return db, idx, sinkidx, nil
}

// utils

var lockFileExistsRe = regexp.MustCompile(`cannot access DB \"(.*)\": lock file \"(.*)\" exists`)

func isLockFileExistsErr(err error) bool {
	log.Println("TODO: check process isn't running")
	if err == nil {
		return false
	}
	errStr := errors.Cause(err).Error()
	if !lockFileExistsRe.MatchString(errStr) {
		return false
	}
	matches := lockFileExistsRe.FindStringSubmatch(errStr)
	if len(matches) == 3 {
		return true
	}
	return false
}

func cleanupLockFiles(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := filepath.Base(path)
		if info.Size() == 0 && len(name) == 41 && name[0] == '.' {
			log.Println("dropping empty lockflile", path)
			if err := os.Remove(path); err != nil {
				return errors.Wrapf(err, "failed to remove %s", name)
			}
		}
		return nil
	})
}
