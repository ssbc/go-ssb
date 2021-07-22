// SPDX-License-Identifier: MIT

package query

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/sroar"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc/v2"
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

func (sp *SubsetPlaner) StreamSubsetQuerySubset(qry SubsetOperation, rxLog margaret.Log, sink *muxrpc.ByteSink) error {
	resulting, err := combineBitmaps(sp, qry)
	if err != nil {
		return err
	}

	if resulting == nil {
		sink.Close()
		return nil
	}

	sink.SetEncoding(muxrpc.TypeJSON)
	// iterate over the combined set of bitmaps
	var (
		it = resulting.NewIterator()

		buf bytes.Buffer
		enc = json.NewEncoder(&buf)
	)

	for it.HasNext() {

		v := it.Next()
		msgv, err := rxLog.Get(margaret.BaseSeq(v))
		if err != nil {
			break
		}

		msg, ok := msgv.(refs.Message)
		if !ok {
			return fmt.Errorf("invalid msg type %T", msgv)
		}

		if true { // TODO: option
			buf.Reset()

			var kv refs.KeyValueRaw
			kv.Key_ = msg.Key()
			kv.Value = *msg.ValueContent()

			if err := enc.Encode(kv); err != nil {
				return fmt.Errorf("failed to encode json: %w", err)
			}

			if _, err = buf.WriteTo(sink); err != nil {
				return fmt.Errorf("failed to send json data: %w", err)
			}
		} else {
			_, err = sink.Write(msg.ValueContentJSON())
			if err != nil {
				return fmt.Errorf("failed to send json data: %w", err)
			}
		}
	}

	sink.Close()
	return nil
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

	case "or":
		var bmap *sroar.Bitmap
		for i, op := range qry.args {
			opsBitmap, err := combineBitmaps(sp, op)
			if err != nil {
				return nil, fmt.Errorf("and operation %d failed: %w", i, err)
			}
			if i == 0 {
				bmap = opsBitmap
			} else {
				bmap.Or(opsBitmap)
			}
		}
		return bmap, nil

	case "and":
		var bmap *sroar.Bitmap
		for i, op := range qry.args {
			opsBitmap, err := combineBitmaps(sp, op)
			if err != nil {
				return nil, fmt.Errorf("or operation %d failed: %w", i, err)
			}
			if i == 0 {
				bmap = opsBitmap
			} else {
				bmap.And(opsBitmap)
			}
		}
		return bmap, nil

	default:
		return nil, fmt.Errorf("sbot: invalid subset query: %s", qry.operation)
	}
}
