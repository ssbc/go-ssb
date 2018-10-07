package multilogs

import (
	"context"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

func OpenUserFeeds(r repo.Interface) (multilog.MultiLog, *badger.DB, func(context.Context, margaret.Log) error, error) {
	return repo.OpenMultiLog(r, "userFeeds", func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		msg, ok := value.(message.StoredMessage)
		if !ok {
			return errors.Errorf("error casting message. got type %T", value)
		}

		authorID := msg.Author.ID
		authorLog, err := mlog.Get(librarian.Addr(authorID))
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = authorLog.Append(seq)
		return errors.Wrap(err, "error appending new author message")
	})
}
