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

func (h getMessagesOfTypeHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink) error {
	var args []struct {
		ID refs.FeedRef

		Type string

		Keys bool
	}
	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	arg := args[0]

	fmt.Printf("by type: %+v\n", arg)

	workSet, err := h.feeds.LoadInternalBitmap(storedrefs.Feed(arg.ID))
	if err != nil {
		// TODO actual assert not found error.
		// fmt.Errorf("failed to load feed %s bitmap: %s", feed.ShortRef(), err.Error())
		snk.Close()
		return nil

	}

	tipeSeqs, err := h.bytype.LoadInternalBitmap(librarian.Addr("string:" + arg.Type))
	if err != nil {
		// return fmt.Errorf("failed to load msg type %s bitmap: %s", tipe, err.Error())
		snk.Close()
		return nil
	}

	snk.SetEncoding(muxrpc.TypeJSON)

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
		if arg.Keys {
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
