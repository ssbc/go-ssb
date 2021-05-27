// SPDX-License-Identifier: MIT

package repo

import (
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"
)

type Interface interface {
	GetPath(...string) string
}

type SimpleIndexMaker interface {
	MakeSimpleIndex(r Interface) (librarian.Index, librarian.SinkIndex, error)
}

type MultiLogMaker interface {
	MakeMultiLog(r Interface) (multilog.MultiLog, librarian.SinkIndex, error)
}

type MakeMultiLog func(r Interface) (multilog.MultiLog, librarian.SinkIndex, error)
type MakeSimpleIndex func(r Interface) (librarian.Index, librarian.SinkIndex, error)
