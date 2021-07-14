// SPDX-License-Identifier: MIT

// Package indexes contains functions to create indexing for 'get(%ref) -> message'.
// Also contains a utility to open the contact trust graph using the repo and graph packages.
package indexes

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"
	libbadger "go.cryptoscope.co/margaret/indexes/badger"

	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

// OpenGet supplies the get(msgRef) -> rootLogSeq idx
func OpenGet(db *badger.DB) (librarian.Index, librarian.SinkIndex) {
	idx := libbadger.NewIndexWithKeyPrefix(db, margaret.BaseSeq(0), []byte("byMsgRef"))
	sinkIdx := librarian.NewSinkIndex(updateFn, idx)
	return idx, sinkIdx
}

func updateFn(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
	msg, ok := val.(refs.Message)
	if !ok {
		err, ok := val.(error)
		if ok && margaret.IsErrNulled(err) {
			return nil
		}
		return fmt.Errorf("index/get: unexpected message type: %T", val)
	}

	err := idx.Set(ctx, storedrefs.Message(msg.Key()), seq.Seq())
	if err != nil {
		return fmt.Errorf("index/get: failed to update message %s (seq: %d): %w", msg.Key().Ref(), seq.Seq(), err)
	}
	return nil
}
