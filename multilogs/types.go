package multilogs

import (
	"context"
	"encoding/json"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

func OpenMessageTypes(r repo.Interface) (multilog.MultiLog, *badger.DB, func(context.Context, margaret.Log) error, error) {
	return repo.OpenMultiLog(r, "msgTypes", func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		msg, ok := value.(message.StoredMessage)
		if !ok {
			return errors.Errorf("error casting message. got type %T", value)
		}

		var typeMsg struct {
			Content struct {
				Type string
			}
		}

		err := json.Unmarshal(msg.Raw, &typeMsg)
		typeStr := typeMsg.Content.Type
		// TODO: maybe check error with more detail - i.e. only drop type errors
		if err != nil || typeStr == "" {
			return nil
		}

		authorLog, err := mlog.Get(librarian.Addr(typeStr))
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = authorLog.Append(seq)
		return errors.Wrapf(err, "error appending message of type %q", typeStr)
	})
}
