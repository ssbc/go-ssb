package replicate

import (
	"context"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
)

type replicatePlug struct {
	h muxrpc.Handler
}

func NewPlug(users multilog.MultiLog) ssb.Plugin {
	plug := &replicatePlug{}
	plug.h = replicateHandler{
		users: users,
	}
	return plug
}

func (lt replicatePlug) Name() string { return "replicate" }

func (replicatePlug) Method() muxrpc.Method {
	return muxrpc.Method{"replicate"}
}
func (lt replicatePlug) Handler() muxrpc.Handler {
	return lt.h
}

type replicateHandler struct {
	users multilog.MultiLog
}

func (g replicateHandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (g replicateHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: add replicate, block, changes
	if len(req.Method) < 2 && req.Method[1] != "upto" {
		req.CloseWithError(errors.Errorf("invalid method"))
		return
	}

	storedFeeds, err := g.users.List()
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "replicate: failed to pump msgs"))
		return
	}

	for _, author := range storedFeeds {
		subLog, err := g.users.Get(author)
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "replicate: failed to pump msgs"))
			return
		}
		authorRef := &ssb.FeedRef{
			Algo: "ed25519",
			ID:   []byte(author),
		}
		currSeq, err := subLog.Seq().Value()
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "replicate: failed to pump msgs"))
			return
		}

		err = req.Stream.Pour(ctx, UpToResponse{
			ID:       authorRef,
			Sequence: currSeq.(margaret.Seq).Seq() + 1})
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "replicate: failed to pump msgs"))
			return
		}

	}

	req.Stream.Close()
}

type UpToResponse struct {
	ID       *ssb.FeedRef `json:"id"`
	Sequence int64        `json:"sequence"`
}
