package repo

import (
	"os"

	"github.com/pkg/errors"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/framing/lengthprefixed"
	"go.cryptoscope.co/margaret/offset"

	"go.cryptoscope.co/sbot/message"
)

func OpenRootLog(r Interface) (margaret.Log, error) {
	logFile, err := os.OpenFile(r.GetPath("log"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "error opening log file")
	}

	// TODO use proper log message type here
	// FIXME: 16kB because some messages are even larger than 12kB - even though the limit is supposed to be 8kb
	rootLog, err := offset.New(logFile, lengthprefixed.New32(16*1024), msgpack.New(&message.StoredMessage{}))
	return rootLog, errors.Wrap(err, "failed to create rootLog")
}
