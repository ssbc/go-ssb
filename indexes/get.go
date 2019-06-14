package indexes

import (
	"context"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

const FolderNameGet = "get"

// OpenGet supplies the get(msgRef) -> rootLogSeq idx
func OpenGet(r repo.Interface) (librarian.Index, repo.ServeFunc, error) {
	db, sinkIdx, serve, err := repo.OpenBadgerIndex(r, FolderNameGet, getIDX)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting get() index")
	}
	nextServe := func(ctx context.Context, log margaret.Log, live bool) error {
		err := serve(ctx, log, live)
		if err != nil {
			return err
		}
		return db.Close()
	}
	return sinkIdx, nextServe, nil
}

func getIDX(db *badger.DB) librarian.SinkIndex {
	idx := libbadger.NewIndex(db, margaret.BaseSeq(0))
	idxSink := librarian.NewSinkIndex(func(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
		msg, ok := val.(message.StoredMessage)
		if !ok {
			return errors.Errorf("index/get: unexpected message type: %T", val)
		}
		err := idx.Set(ctx, librarian.Addr(msg.Key.Hash), seq.Seq())
		return errors.Wrapf(err, "index/get: failed to update message %s (seq: %d)", msg.Key.Ref(), seq.Seq())
	}, idx)
	return idxSink
}
