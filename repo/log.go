package repo

import (
	"os"

	"github.com/pkg/errors"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/framing/lengthprefixed"
	"go.cryptoscope.co/margaret/offset"

	"go.cryptoscope.co/ssb/message"
)

func OpenLog(r Interface, path ...string) (margaret.Log, error) {
	// prefix path with "logs" if path is not empty, otherwise use "log"
	path = append([]string{"log"}, path...)
	if len(path) > 1 {
		path[0] = "logs"
	}

	logFile, err := os.OpenFile(r.GetPath(path...), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "error opening log file")
	}

	// TODO use proper log message type here
	// FIXME: 16kB because some messages are even larger than 12kB - even though the limit is supposed to be 8kb
	log, err := offset.New(logFile, lengthprefixed.New32(16*1024), msgpack.New(&message.StoredMessage{}))
	return log, errors.Wrap(err, "failed to open log")
}
