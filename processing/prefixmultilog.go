package processing

import (
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
)

type PrefixMultilog struct {
	mlog   multilog.MultiLog
	prefix string
}

func (mlog PrefixMultilog) Get(addr librarian.Addr) (margaret.Log, error) {
	return mlog.mlog.Get(librarian.Addr(encodeStrings(nil, mlog.prefix, string(addr))))
}

func (mlog PrefixMultilog) List() ([]librarian.Addr, error) {
	list, err := mlog.mlog.List()

	for i := range list {
		list[i] = librarian.Addr(encodeStrings(nil, mlog.prefix, string(list[i])))
	}

	return list, err
}

func (mlog PrefixMultilog) Delete(addr librarian.Addr) error {
	return mlog.mlog.Delete(librarian.Addr(encodeStrings(nil, mlog.prefix, string(addr))))
}

func (mlog PrefixMultilog) Close() error {
	return mlog.mlog.Close()
}
