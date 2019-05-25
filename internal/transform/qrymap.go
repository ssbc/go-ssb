package transform

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/legacy"
)

type KeyValue struct {
	Message ssb.Message
	Data    json.RawMessage
}

func NewKeyValueWrapper(src luigi.Source, wrap bool) luigi.Source {
	return mfr.SourceMap(src, func(ctx context.Context, v interface{}) (interface{}, error) {
		abs, ok := v.(ssb.Message)
		if !ok {
			return nil, errors.Errorf("kvwrap: wrong message type. expected %T - got %T", abs, v)
		}

		if !wrap {
			return &KeyValue{
				Message: abs,
				Data:    abs.ValueContentJSON(),
			}, nil
		}

		var kv legacy.KeyValueRaw
		kv.Key = abs.Key()
		kv.Value = abs.ValueContentJSON()
		if sm, ok := v.(legacy.StoredMessage); ok {
			kv.Timestamp = sm.Timestamp_.UnixNano() / 1000000
		}
		kvMsg, err := json.Marshal(kv)
		if err != nil {
			return nil, errors.Wrapf(err, "kvwrap: failed to k:v map message")
		}
		return &KeyValue{
			Message: abs,
			Data:    kvMsg,
		}, nil

	})
}
