// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package repo

import (
	"github.com/dgraph-io/badger/v3"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"
)

// Interface is the basic interface for working with paths for file locations in a single repository. blobs, log, secret, you name it.
type Interface interface {
	GetPath(...string) string
}

// KeyValueIndexMaker is an sbot facility to creating dynamically monunted k-v indexes
type KeyValueIndexMaker interface {
	MakeKeyValueIndex(db *badger.DB) (librarian.Index, librarian.SinkIndex, error)
}

// MakeKeyValueIndex is the "functional" version of a KeyValueIndexMaker
type MakeKeyValueIndex func(db *badger.DB) (librarian.Index, librarian.SinkIndex, error)

// MultiLogMaker is an sbot facility to creating dynamically monunted multilog indexes
type MultiLogMaker interface {
	MakeMultiLogIndex(db *badger.DB) (multilog.MultiLog, librarian.SinkIndex, error)
}

// MakeMultiLogIndex is the "functional" version of a MultiLogMaker
type MakeMultiLogIndex func(db *badger.DB) (multilog.MultiLog, librarian.SinkIndex, error)
