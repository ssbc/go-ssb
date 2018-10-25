package repo

import (
	"strings"

	"github.com/pkg/errors"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/offset2"

	"go.cryptoscope.co/ssb/message"
)

func OpenLog(r Interface, path ...string) (margaret.Log, error) {
	// prefix path with "logs" if path is not empty, otherwise use "log"
	path = append([]string{"log"}, path...)
	if len(path) > 1 {
		path[0] = "logs"
	}

	// TODO use proper log message type here
	log, err := offset2.Open(strings.Join(path, "/"), msgpack.New(&message.StoredMessage{}))
	return log, errors.Wrap(err, "failed to open log")
}
