// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// Package indexes contains functions to create indexing for 'get(%ref) -> message'.
// Also contains a utility to open the contact trust graph using the repo and graph packages.
package indexes

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/ssbc/margaret"
	librarian "github.com/ssbc/margaret/indexes"
	libbadger "github.com/ssbc/margaret/indexes/badger"

	"github.com/ssbc/go-ssb/internal/storedrefs"
	refs "github.com/ssbc/go-ssb-refs"
)

// OpenGet supplies the get(msgRef) -> rootLogSeq idx
func OpenGet(db *badger.DB) (librarian.Index, librarian.SinkIndex) {
	idx := libbadger.NewIndexWithKeyPrefix(db, int64(0), []byte("byMsgRef"))
	sinkIdx := librarian.NewSinkIndex(updateGetFn, idx)
	return idx, sinkIdx
}

func updateGetFn(ctx context.Context, seq int64, val interface{}, idx librarian.SetterIndex) error {
	msg, ok := val.(refs.Message)
	if !ok {
		err, ok := val.(error)
		if ok && margaret.IsErrNulled(err) {
			return nil
		}
		return fmt.Errorf("index/get: unexpected message type: %T", val)
	}

	err := idx.Set(ctx, storedrefs.Message(msg.Key()), seq)
	if err != nil {
		return fmt.Errorf("index/get: failed to update message %s (seq: %d): %w", msg.Key().String(), seq, err)
	}
	return nil
}
