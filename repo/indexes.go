package repo

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dgraph-io/badger"
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
		return nil, nil, fmt.Errorf("openIndex: error making index directory: %w", err)
	}

	db, err := OpenMKV(pth)
	if err != nil {
		return nil, nil, fmt.Errorf("openIndex: failed to open MKV database: %w", err)
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
			return nil, fmt.Errorf("failed to create mkv: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat path location: %w", err)
	} else {
		db, err = kv.Open(dbname, opts)
		if err != nil {

			if !isLockFileExistsErr(err) {
				return nil, err
			}
			if err := cleanupLockFiles(pth); err != nil {
				return nil, fmt.Errorf("failed to recover lockfiles: %w", err)
			}
			db, err = kv.Open(dbname, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to open mkv: %w", err)
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
		return nil, nil, nil, fmt.Errorf("error making index directory: %w", err)
	}

	db, err := badger.Open(badgerOpts(pth))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("db/idx: badger failed to open: %w", err)
	}

	idx, sinkidx := f(db)

	return db, idx, sinkidx, nil
}

// utils

var lockFileExistsRe = regexp.MustCompile(`cannot access DB \"(.*)\": lock file \"(.*)\" exists`)

// TODO: add test
func isLockFileExistsErr(err error) bool {
	log.Println("TODO: check process isn't running")
	return false
	if err == nil {
		return false
	}

	// TODO: extract core error?
	errStr := err.Error()
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
				return fmt.Errorf("failed to remove %s: %w", name, err)
			}
		}
		return nil
	})
}
