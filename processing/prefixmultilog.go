package processing

import (
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
)

// PrefixMultilog wraps an existing multilog and prepends the address of
// every sublog with prefix.
//
// The prefix is prepended using an internally defined encoding, to make
// sure that e.g. the sublog "def", prefixed with string "abc" does not
// collide with the sublog "ef", prefixed with the string "abcd".
type PrefixMultilog struct {
	MLog   multilog.MultiLog
	Prefix string
}

// Get returns the sublog addr, prefixed with Prefix.
func (mlog PrefixMultilog) Get(addr librarian.Addr) (margaret.Log, error) {
	return mlog.MLog.Get(librarian.Addr(encodeStrings(nil, mlog.Prefix, string(addr))))
}

// List returns the list of all sublogs
func (mlog PrefixMultilog) List() ([]librarian.Addr, error) {
	list, err := mlog.MLog.List()

	for i := range list {
		list[i] = librarian.Addr(encodeStrings(nil, mlog.Prefix, string(list[i])))
	}

	return list, err
}

// Delete delets a sublog from the multilog
func (mlog PrefixMultilog) Delete(addr librarian.Addr) error {
	return mlog.MLog.Delete(librarian.Addr(encodeStrings(nil, mlog.Prefix, string(addr))))
}

// Close closes the underlying multilog
func (mlog PrefixMultilog) Close() error {
	return mlog.MLog.Close()
}
