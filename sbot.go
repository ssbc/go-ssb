package ssb

import (
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
)

type Publisher interface {
	margaret.Log

	// Publish is a utility wrapper around append which returns the new message reference key
	Publish(content interface{}) (*MessageRef, error)
}

type Getter interface {
	Get(MessageRef) (Message, error)
}

type MultiLogGetter interface {
	GetMultiLog(name string) (multilog.MultiLog, bool)
}

type SimpleIndexGetter interface {
	GetSimpleIndex(name string) (librarian.Index, bool)
}

type Indexer interface {
	MultiLogGetter
	SimpleIndexGetter
	GetIndexNamesSimple() []string
	GetIndexNamesMultiLog() []string
}

type Statuser interface {
	Status() (*Status, error)
}

type PeerStatus struct {
	Addr  string
	Since string
}
type Status struct {
	Peers   []PeerStatus
	Blobs   interface{}
	Root    margaret.Seq
	Indexes struct {
		Simple   map[string]int64
		MultiLog map[string]int64
	}
}
