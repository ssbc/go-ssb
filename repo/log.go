package repo

import (
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/sqlite"
	"go.cryptoscope.co/ssb/message/multimsg"
)

func OpenLog(r Interface, path ...string) (margaret.Log, error) {
	// prefix path with "logs" if path is not empty, otherwise use "log"
	path = append([]string{"log"}, path...)
	if len(path) > 1 {
		path[0] = "logs"
	}

	// log, err := offset2.Open(r.GetPath(path...), msgpack.New(&multimsg.MultiMessage{}))
	log, err := sqlite.Open(r.GetPath(path...), msgpack.New(&multimsg.MultiMessage{}))
	return multimsg.NewWrappedLog(log), errors.Wrap(err, "failed to open log")
}
