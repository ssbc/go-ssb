package multilog

import (
	"io"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
)

// MultiLog is a collection of logs, keyed by a librarian.Addr
// TODO maybe only call this log to avoid multilog.MultiLog?
type MultiLog interface {
	Get(librarian.Addr) (margaret.Log, error)
	List() ([]librarian.Addr, error)
	io.Closer
}

func Has(mlog MultiLog, addr librarian.Addr) (bool, error) {
	slog, err := mlog.Get(addr)
	if err != nil {
		return false, err
	}

	seqVal, err := slog.Seq().Value()
	if err != nil {
		return false, err
	}

	return seqVal.(margaret.BaseSeq) != margaret.SeqEmpty, nil
}
