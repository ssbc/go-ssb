package processing

/* Some thoughts on this:

Firstly, I think it should be renamed to NamespacedMultilog or so,
because the prefixing is a technique to achieve namespacing, and in fact
we do more than just dumb prefixing to make it work properly.

Secondly, this should probably live somewhere in
"go.cryptoscope.co/margaret/multilog" or so.

Thirdly, we need to look at the tradeoffs this brings, and what the benefits
of doing it this way are. Without having benchmarked, my hunch is that
> combining multiple multilogs into a single namespaced one reduces
> memory consumption, because per-multilog overhead is avoided

however, this probably comes at a cost, because
> having several multilogs be backed by a single one that needs locking
>means that there only can be one writer at a time.

Figuring out which approach is best needs to be done through benchmarks,
but for now we'll just use seperate ones.
*/

/*

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
*/
