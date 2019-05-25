package multilogs

import (
	"context"
	"encoding/json"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

const IndexNameTypes = "msgTypes"

func OpenMessageTypes(r repo.Interface) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, IndexNameTypes, func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := value.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}
		msg, ok := value.(ssb.Message)
		if !ok {
			err := errors.Errorf("error casting message. got type %T", value)
			// fmt.Println(err)
			return err
		}

		var typeMsg struct {
			Type string
		}

		err := json.Unmarshal(msg.ContentBytes(), &typeMsg)
		typeStr := typeMsg.Type
		// TODO: maybe check error with more detail - i.e. only drop type errors
		if err != nil || typeStr == "" {
			return nil
		}

		typedLog, err := mlog.Get(librarian.Addr(typeStr))
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = typedLog.Append(seq)
		return errors.Wrapf(err, "error appending message of type %q", typeStr)
	})
}
