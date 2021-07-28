// SPDX-License-Identifier: MIT

package query

import (
	"fmt"

	"github.com/dgraph-io/sroar"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/ssb/internal/storedrefs"
)

type SubsetPlaner struct {
	authors, bytype *roaring.MultiLog
}

func NewSubsetPlaner(authors, bytype *roaring.MultiLog) *SubsetPlaner {
	return &SubsetPlaner{
		authors: authors,
		bytype:  bytype,
	}
}

// QuerySubsetBitmap evaluates the passed SubsetOperation and returns a bitmap which maps to messages in the receive log.
func (sp *SubsetPlaner) QuerySubsetBitmap(qry SubsetOperation) (*sroar.Bitmap, error) {
	return combineBitmaps(sp, qry)
}

// QuerySubsetMessageRefs evaluates the passed SubsetOperation and returns a slice of message references
func (sp *SubsetPlaner) QuerySubsetMessageRefs(rxLog margaret.Log, qry SubsetOperation) ([]refs.MessageRef, error) {
	resulting, err := combineBitmaps(sp, qry)
	if err != nil {
		return nil, err
	}

	if resulting == nil {
		return nil, nil
	}

	// iterate over the combined set of bitmaps
	it := resulting.NewIterator()

	var msgs []refs.MessageRef

	for it.HasNext() {

		v := it.Next()
		msgv, err := rxLog.Get(margaret.BaseSeq(v))
		if err != nil {
			return nil, err
		}

		msg, ok := msgv.(refs.Message)
		if !ok {
			return nil, fmt.Errorf("invalid msg type %T", msgv)
		}

		msgs = append(msgs, msg.Key())
	}

	return msgs, nil
}

func combineBitmaps(sp *SubsetPlaner, qry SubsetOperation) (*sroar.Bitmap, error) {
	switch qry.operation {

	case "author":
		return sp.authors.LoadInternalBitmap(storedrefs.Feed(*qry.feed))

	case "type":
		return sp.bytype.LoadInternalBitmap(indexes.Addr("string:" + qry.string))

	case "or", "and":
		// the bitmap all operations are applied onto
		var workBitmap *sroar.Bitmap

		for i, op := range qry.args {

			// get the bitmap for the current operation
			opsBitmap, err := combineBitmaps(sp, op)
			if err != nil {
				return nil, fmt.Errorf("boolean (%s) operation %d of %d failed: %w",
					qry.operation,
					i,
					len(qry.args),
					err,
				)
			}

			if i == 0 { // first op in the array just sets the map
				workBitmap = opsBitmap
			} else {
				// now apply the operation either OR or AND
				// to the previous working bitmap
				if qry.operation == "or" {
					workBitmap.Or(opsBitmap)
				} else {
					workBitmap.And(opsBitmap)
				}
			}
		}
		return workBitmap, nil

	default:
		return nil, fmt.Errorf("sbot: invalid subset query: %s", qry.operation)
	}
}
