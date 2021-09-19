// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package repo

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dgraph-io/badger/v3"
	librarian "go.cryptoscope.co/margaret/indexes"
	libbadger "go.cryptoscope.co/margaret/indexes/badger"
)

// PrefixIndex is the default namespace for simple/key-value based indexes
const PrefixIndex = "indexes"

// OpenIndex returns an initialzed simple/key-value based indexes, using the passed badger database for storage.
func OpenIndex(db *badger.DB, name string, f func(librarian.SeqSetterIndex) librarian.SinkIndex) (librarian.Index, librarian.SinkIndex, error) {
	seqSetter := libbadger.NewIndexWithKeyPrefix(db, 0, []byte("index"+name))
	return seqSetter, f(seqSetter), nil
}

// KeyValueIndexCreater creates a new key-value based index
type KeyValueIndexCreater func(*badger.DB) (librarian.SeqSetterIndex, librarian.SinkIndex)

// OpenBadgerIndex creates a new badger database to back a new key-value style index
func OpenBadgerIndex(r Interface, name string, f KeyValueIndexCreater) (*badger.DB, librarian.SeqSetterIndex, librarian.SinkIndex, error) {
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
	if err == nil {
		return false
	}

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
