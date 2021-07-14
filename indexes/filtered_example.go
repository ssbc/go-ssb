package indexes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"
	libbadger "go.cryptoscope.co/margaret/indexes/badger"
	refs "go.mindeco.de/ssb-refs"
)

// OpenExample
func OpenExample(db *badger.DB) (librarian.Index, librarian.SinkIndex) {
	idx := libbadger.NewIndexWithKeyPrefix(db, margaret.BaseSeq(0), []byte("index-example"))
	sinkIdx := librarian.NewSinkIndex(updateExampleFn, idx)
	return idx, sinkIdx
}

func updateExampleFn(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
	msg, ok := val.(refs.Message)
	if !ok {
		err, ok := val.(error)
		if ok && margaret.IsErrNulled(err) {
			return nil
		}
		return fmt.Errorf("index/get: unexpected message type: %T", val)
	}

	var p struct {
		Type string
	}
	err := json.Unmarshal(msg.ContentBytes(), &p)
	if err != nil {
		fmt.Println("exmaple index unmarshal failed", err)
		return nil
	}

	fmt.Println("processing", seq.Seq())
	fmt.Println("type", p.Type, "from", msg.Author().Ref(), msg.Seq())

	// err := idx.Set(ctx, storedrefs.Message(msg.Key()), seq.Seq())
	// if err != nil {
	// 	return fmt.Errorf("index/get: failed to update message %s (seq: %d): %w", msg.Key().Ref(), seq.Seq(), err)
	// }
	return nil
}
