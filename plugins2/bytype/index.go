// SPDX-License-Identifier: MIT

package bytype

import (
	"context"
	"encoding/json"

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
	plug.h.types = mlog
	return mlog, serve, err
}
