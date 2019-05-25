package multilogs

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
)

const IndexNamePrivates = "privates"

// not strictly a multilog but allows multiple keys and gives us the good resumption
func OpenPrivateRead(log kitlog.Logger, r repo.Interface, kp *ssb.KeyPair) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, IndexNamePrivates, func(ctx context.Context, seq margaret.Seq, val interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := val.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}

		msg, ok := val.(ssb.Message)
		if !ok {
			err := errors.Errorf("error casting message. got type %T", val)
			fmt.Println("privateIDX failed:", err)
			return err
		}

		if _, err := private.Unbox(kp, string(msg.ContentBytes())); err != nil {
			return nil
		}

		userPrivs, err := mlog.Get(kp.Id.StoredAddr())
		if err != nil {
			return errors.Wrap(err, "error opening priv sublog")
		}

		_, err = userPrivs.Append(seq.Seq())
		return errors.Wrapf(err, "error appending PM for %s", kp.Id.Ref())
	})
}
