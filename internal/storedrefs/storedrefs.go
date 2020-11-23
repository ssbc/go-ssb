// Package storedrefs provides methods to encode certain types as bytes, as used by the internal storage system.
package storedrefs

import (
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

// Feed returns the key under which this ref is stored in the indexing system
func Feed(r *refs.FeedRef) librarian.Addr {
	sr, err := tfk.FeedFromRef(r)
	if err != nil {
		panic(errors.Wrap(err, "failed to make stored ref"))
	}

	b, err := sr.MarshalBinary()
	if err != nil {
		panic(errors.Wrap(err, "error while marshalling stored ref"))
	}
	return librarian.Addr(b)
}
