package multilogs

import (
	"context"
	"encoding/json"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret/multilog"

	"github.com/dgraph-io/badger"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
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

		msg := val.(message.StoredMessage)
		var dmsg struct {
			Content interface{} `json:"content"`
		}

		if err := json.Unmarshal(msg.Raw, &dmsg); err != nil {
			return errors.Wrap(err, "db/idx about: first json unmarshal failed")
		}

		privstr, ok := dmsg.Content.(string)
		if !ok {
			// skip everything that isn't a string
			return nil
		}

		if _, err := private.Unbox(kp, privstr); err != nil {
			return nil
		}

		userPrivs, err := mlog.Get(librarian.Addr(kp.Id.ID))
		if err != nil {
			return errors.Wrap(err, "error opening priv sublog")
		}

		_, err = userPrivs.Append(seq.Seq())
		return errors.Wrapf(err, "error appending PM for %s", kp.Id.Ref())
	})
}
