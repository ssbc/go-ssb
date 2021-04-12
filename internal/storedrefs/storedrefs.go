// Package storedrefs provides methods to encode certain types as bytes, as used by the internal storage system.
package storedrefs

import (
	"fmt"

	"go.cryptoscope.co/librarian"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

// Feed returns the key under which this ref is stored in the indexing system
func Feed(r refs.FeedRef) librarian.Addr {
	sr, err := tfk.FeedFromRef(r)
	if err != nil {
		panic(fmt.Errorf("failed to make stored ref: %w", err))
	}

	b, err := sr.MarshalBinary()
	if err != nil {
		panic(fmt.Errorf("error while marshalling stored ref: %w", err))
	}
	return librarian.Addr(b)
}

// Message returns the key under which this ref is stored in the indexing system
func Message(r refs.MessageRef) librarian.Addr {
	sr, err := tfk.MessageFromRef(r)
	if err != nil {
		panic(fmt.Errorf("failed to make stored ref: %w", err))
	}

	b, err := sr.MarshalBinary()
	if err != nil {
		panic(fmt.Errorf("error while marshalling stored ref: %w", err))
	}
	return librarian.Addr(b)
}

// TangleV1 show how we encode v1 (nameless) tangles for the storage layer
func TangleV1(r refs.MessageRef) librarian.Addr {
	var addr = make([]byte, 3+32)
	copy(addr[0:3], []byte("v1:"))
	r.CopyHashTo(addr[3:])
	return librarian.Addr(addr)
}

// TangleV2 show how we encode v2 (named) tangles for the storage layer
func TangleV2(name string, r refs.MessageRef) librarian.Addr {
	var addr = make([]byte, 4+32+len(name))
	copy(addr, []byte("v2:"+name+":"))
	r.CopyHashTo(addr[4+len(name):])
	return librarian.Addr(addr)
}
