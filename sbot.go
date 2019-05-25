package ssb

import (
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
