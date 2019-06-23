package multilogs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

const IndexKeyValue = "keyValue"

func OpenKeyValue(r repo.Interface) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, IndexNameFeeds, func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := value.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}

		msg, ok := value.(message.StoredMessage)
		if !ok {
			return errors.Errorf("error casting message. got type %T", value)
		}

		// string content means encrypted message; skip
		if msg.Raw[0] == '"' {
			return nil
		}

		// now we should only see object messages and everything else is an error

		var contentMsg struct {
			Content map[string]interface{} `json:"content"`
		}

		err := json.Unmarshal(msg.Raw, &contentMsg)
		if err != nil {
			return errors.Wrap(err, "error decoding message")
		}

		var errs []error

		for k, v := range contentMsg.Content {
			str, ok := v.(string)
			if !ok {
				continue
			}
			id, err := encodeStringTuple(k, str)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			kvLog, err := mlog.Get(id)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			_, err = kvLog.Append(seq)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}

		// TODO use proper multierror thing here
		if len(errs) > 0 {
			var acc string
			for _, e := range errs {
				acc += fmt.Sprintf("\n - %s", e.Error())
			}
			return errors.Errorf("%d errors occurred:%s", len(errs), acc)
		}

		return nil
	})
}

// encodeStringTuple encodes a pair of strings to bytes by length-prefixing and then concatenating them. Returns an error if either input string is longer than 255 bytes.
func encodeStringTuple(str1, str2 string) (librarian.Addr, error) {
	var (
		bs1, bs2 = []byte(str1), []byte(str2)
		l1, l2   = len(bs1), len(bs2)
		buf      = make([]byte, l1+l2+2)
	)

	fmtStr := "could not encode string tuple: %s string too long (%d>255)"

	if l1 > 255 {
		return "", errors.Errorf(fmtStr, "first", l1)
	}

	if l2 > 255 {
		return "", errors.Errorf(fmtStr, "second", l2)
	}

	buf[0] = byte(l1)
	buf[l1+1] = byte(l2)

	copy(buf[1:], bs1)
	copy(buf[2+l1:], bs2)

	return librarian.Addr(buf), nil
}
