package multilogs

import (
	"context"
	"encoding/json"

	"go.cryptoscope.co/ssb"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

const IndexNameTangles = "tangles"

func OpenTangles(r repo.Interface) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, IndexNameTangles, func(ctx context.Context, seq margaret.Seq, msgv interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := msgv.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}
		msg, ok := msgv.(message.StoredMessage)
		if !ok {
			return errors.Errorf("error casting message. got type %T", msgv)
		}

		var value struct {
			Content struct {
				Root *ssb.MessageRef
			}
		}

		err := json.Unmarshal(msg.Raw, &value)
		// TODO: maybe check error with more detail - i.e. only drop type errors
		if err != nil || value.Content.Root == nil {
			return nil
		}

		tangleLog, err := mlog.Get(librarian.Addr(value.Content.Root.Hash))
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = tangleLog.Append(seq)
		// log.Println(msg.Key.Ref(), value.Content.Root.Ref(), seq)
		return errors.Wrapf(err, "error appending root message:", msg.Key.Ref())
	})

}
