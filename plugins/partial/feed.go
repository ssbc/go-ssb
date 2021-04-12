package partial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins/gossip"
)

//  https://ssbc.github.io/scuttlebutt-protocol-guide/#createHistoryStream

type getFeedHandler struct {
	fm *gossip.FeedManager
}

func (h getFeedHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink) error {
	var args []json.RawMessage
	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		return err
	}

	if len(args) < 1 {
		return errors.New("partial/feed: not enough arguments, expecting 1 object")
	}

	var query message.CreateHistArgs
	err = json.Unmarshal(args[0], &query)
	if err != nil {
		return err
	}

	if query.Limit == 0 { // stupid unset default
		query.Limit = -1
	}

	fmt.Printf("partial/feed (forward): %s:%d\n", query.ID.Ref(), query.Seq)
	// fmt.Printf("partial/feed: full query %+v\n", query)
	fmt.Println(string(args[0]))

	snk.SetEncoding(muxrpc.TypeJSON)

	err = h.fm.CreateStreamHistory(ctx, snk, query)
	if err != nil {
		if luigi.IsEOS(err) {
			req.Stream.Close()
			return nil
		}
		return fmt.Errorf("partial/feed failed: %w", err)
	}
	return nil
}

type getFeedReverseHandler struct {
	fm *gossip.FeedManager
}

func (h getFeedReverseHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink) error {
	var args []json.RawMessage
	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		return err
	}

	if len(args) < 1 {
		return errors.New("partial/feed: not enough arguments, expecting 1 object")
	}

	var query message.CreateHistArgs
	err = json.Unmarshal(args[0], &query)
	if err != nil {
		return err
	}

	if query.Limit == 0 { // stupid unset default
		query.Limit = -1
	}
	query.Reverse = true

	fmt.Printf("partial/feed (reverse): %s:%d\n", query.ID.Ref(), query.Seq)
	// fmt.Printf("partial/feed: full query %+v\n", query)
	fmt.Println(string(args[0]))

	snk.SetEncoding(muxrpc.TypeJSON)

	err = h.fm.CreateStreamHistory(ctx, snk, query)
	if err != nil {
		if luigi.IsEOS(err) {
			req.Stream.Close()
			return nil
		}
		return fmt.Errorf("partial/feed reverse failed: %w", err)
	}
	return nil
}
