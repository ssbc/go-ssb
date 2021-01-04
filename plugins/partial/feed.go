package partial

import (
	"context"
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

func (h getFeedHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink, edp muxrpc.Endpoint) error {
	args := req.Args()

	if len(args) < 1 {
		return errors.New("ssb/message: not enough arguments, expecting feed id")

	}
	argMap, ok := args[0].(map[string]interface{})
	if !ok {
		err := fmt.Errorf("ssb/message: not the right map - %T", args[0])
		return err

	}
	query, err := message.NewCreateHistArgsFromMap(argMap)
	if err != nil {
		return fmt.Errorf("bad request: %w", err)
	}

	// query.Limit = 50
	// spew.Dump(query)
	err = h.fm.CreateStreamHistory(ctx, snk, query)
	if err != nil {
		if luigi.IsEOS(err) {
			req.Stream.Close()
			return nil
		}
		return fmt.Errorf("createHistoryStream failed: %w", err)
	}
	return nil
}

type getFeedReverseHandler struct {
	fm *gossip.FeedManager
}

func (h getFeedReverseHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink, edp muxrpc.Endpoint) error {
	args := req.Args()

	if len(args) < 1 {
		return errors.New("ssb/message: not enough arguments, expecting feed id")

	}
	argMap, ok := args[0].(map[string]interface{})
	if !ok {
		err := fmt.Errorf("ssb/message: not the right map - %T", args[0])
		return err

	}
	query, err := message.NewCreateHistArgsFromMap(argMap)
	if err != nil {
		return fmt.Errorf("bad request: %w", err)
	}
	query.Reverse = true

	// query.Limit = 50
	// spew.Dump(query)
	err = h.fm.CreateStreamHistory(ctx, snk, query)
	if err != nil {
		if luigi.IsEOS(err) {
			req.Stream.Close()
			return nil
		}
		return fmt.Errorf("createHistoryStream failed: %w", err)
	}
	return nil
}
