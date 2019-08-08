package transform

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"
	"go.cryptoscope.co/ssb"
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

		var kv ssb.KeyValueRaw
		kv.Key_ = abs.Key()
		kv.Value = *abs.ValueContent()
		kv.Timestamp = abs.Received().Unix() * 1000
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
