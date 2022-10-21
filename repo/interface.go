// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package repo

import (
	"github.com/dgraph-io/badger/v3"
	librarian "github.com/ssbc/margaret/indexes"
	"github.com/ssbc/margaret/multilog"
)

type Interface interface {
	GetPath(...string) string
}

type SimpleIndexMaker interface {
	MakeSimpleIndex(db *badger.DB) (librarian.Index, librarian.SinkIndex, error)
}

type MultiLogMaker interface {
	MakeMultiLog(db *badger.DB) (multilog.MultiLog, librarian.SinkIndex, error)
}

type MakeMultiLog func(db *badger.DB) (multilog.MultiLog, librarian.SinkIndex, error)
type MakeSimpleIndex func(db *badger.DB) (librarian.Index, librarian.SinkIndex, error)
