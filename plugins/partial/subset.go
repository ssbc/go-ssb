package partial

import (
	"context"
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb/query"
)

type getSubsetHandler struct {
	queryPlaner *query.SubsetPlaner

	rxLog margaret.Log
}

func (h getSubsetHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk *muxrpc.ByteSink) error {

	var (
		args []query.SubsetOperation
		arg  query.SubsetOperation
	)

	err := json.Unmarshal(req.RawArgs, &args)
	if err != nil {
		return err
	}
	if n := len(args); n != 1 {
		return fmt.Errorf("expected one arguemnt got %d", n)
	}
	arg = args[0]

	err = h.queryPlaner.StreamSubsetQuerySubset(arg, h.rxLog, snk)
	if err != nil {
		return fmt.Errorf("failed to send query result to peer: %w", err)
	}
	return nil
}
