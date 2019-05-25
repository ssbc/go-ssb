package multilogs

import (
	"context"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

const IndexNameFeeds = "userFeeds"

func OpenUserFeeds(r repo.Interface) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, IndexNameFeeds, func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := value.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}

		abstractMsg, ok := value.(ssb.Message)
		if !ok {
			return errors.Errorf("error casting message. got type %T", value)
		}

		author := abstractMsg.Author()
		authorLog, err := mlog.Get(author.StoredAddr())
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = authorLog.Append(seq)
		return errors.Wrap(err, "error appending new author message")
	})
}
