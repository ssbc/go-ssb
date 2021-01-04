package partial

import (
	"context"
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

type getMessagesOfTypeHandler struct {
	rxlog margaret.Log

	feeds  *roaring.MultiLog
	bytype *roaring.MultiLog
}

func (h getMessagesOfTypeHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink, edp muxrpc.Endpoint) error {
	if len(req.Args()) < 1 {
		return fmt.Errorf("invalid arguments")
	}
	var (
		feed *refs.FeedRef
		tipe string

		keys bool

		err error
	)
	switch v := req.Args()[0].(type) {

	case map[string]interface{}:
		refV, ok := v["id"]
		if !ok {
			return fmt.Errorf("invalid argument - missing 'id' in map")
		}

		ref, ok := refV.(string)
		if !ok {
			return fmt.Errorf("invalid argument - 'id' field is not a string")
		}

		feed, err = refs.ParseFeedRef(ref)
		if err != nil {
			return fmt.Errorf("invalid argument: %w", err)
		}

		typeV, ok := v["type"]
		if !ok {
			return fmt.Errorf("invalid argument - missing 'type' in map")
		}

		tipe = typeV.(string)

		if keysV, has := v["keys"]; has {
			if yes, ok := keysV.(bool); ok {
				keys = yes
			}
		}

	default:
		return fmt.Errorf("invalid argument type %T", req.Args()[0])

	}

	workSet, err := h.feeds.LoadInternalBitmap(storedrefs.Feed(feed))
	if err != nil {
		// TODO actual assert not found error.
		// fmt.Errorf("failed to load feed %s bitmap: %s", feed.ShortRef(), err.Error())
		snk.Close()
		return nil

	}

	tipeSeqs, err := h.bytype.LoadInternalBitmap(librarian.Addr("string:" + tipe))
	if err != nil {
		// return fmt.Errorf("failed to load msg type %s bitmap: %s", tipe, err.Error())
		snk.Close()
		return nil

	}

	// which sequences are in both?
	workSet.And(tipeSeqs)

	it := workSet.Iterator()
	for it.HasNext() {

		v := it.Next()
		msgv, err := h.rxlog.Get(margaret.BaseSeq(v))
		if err != nil {
			break
		}

		msg, ok := msgv.(refs.Message)
		if !ok {
			return fmt.Errorf("invalid msg type %T", msgv)
		}
		if keys {
			var kv refs.KeyValueRaw
			kv.Key_ = msg.Key()
			kv.Value = *msg.ValueContent()
			b, err := json.Marshal(kv)
			if err != nil {
				return fmt.Errorf("failed to encode json: %w", err)
			}
			_, err = snk.Write(json.RawMessage(b))
			if err != nil {
				return fmt.Errorf("failed to send json data: %w", err)
			}
		} else {
			_, err = snk.Write(msg.ValueContentJSON())
			if err != nil {
				return fmt.Errorf("failed to send json data: %w", err)
			}
		}
	}
	snk.Close()
	return nil
}
