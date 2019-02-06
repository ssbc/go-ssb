package transform

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"
	"go.cryptoscope.co/ssb/message"
)

func NewKeyValueWrapper(src luigi.Source, wrap bool) luigi.Source {
	return mfr.SourceMap(src, func(ctx context.Context, v interface{}) (interface{}, error) {
		storedMsg, ok := v.(message.StoredMessage)
		if !ok {
			return nil, errors.Errorf("wrong message type. expected %T - got %T", storedMsg, v)
		}
		if !wrap {
			return storedMsg.Raw, nil
		}
		var kv message.KeyValueRaw
		kv.Key = storedMsg.Key
		kv.Value = storedMsg.Raw
		kv.Timestamp = storedMsg.Timestamp.UnixNano() / 1000
		kvMsg, err := json.Marshal(kv)
		if err != nil {
			return nil, errors.Wrapf(err, "rootLog: failed to k:v map message")
		}
		return kvMsg, nil
	})
}
