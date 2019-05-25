package tangles

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

func (plug *Plugin) MakeMultiLog(r repo.Interface) (multilog.MultiLog, repo.ServeFunc, error) {
	mlog, serve, err := repo.OpenMultiLog(r, plug.Name(), func(ctx context.Context, seq margaret.Seq, msgv interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := msgv.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}

		msg, ok := msgv.(ssb.Message)
		if !ok {
			err := errors.Errorf("error casting message. got type %T", msgv)
			fmt.Println("tangleIDX failed:", err)
			return err
		}

		var value struct {
			Root *ssb.MessageRef
		}

		err := json.Unmarshal(msg.ContentBytes(), &value)
		// TODO: maybe check error with more detail - i.e. only drop type errors
		if err != nil || value.Root == nil {
			return nil
		}

		tangleLog, err := mlog.Get(librarian.Addr(value.Root.Hash))
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = tangleLog.Append(seq)
		// log.Println(msg.Key.Ref(), value.Root.Ref(), seq)
		return errors.Wrapf(err, "error appending root message %v", msg.Key())
	})
	plug.h.tangle = mlog
	return mlog, serve, err
}
