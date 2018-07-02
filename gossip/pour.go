package gossip

import (
	"context"
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/message"
)

func (hist *Handler) pourFeed(ctx context.Context, req *muxrpc.Request) error {
	// check & parse args
	qv := req.Args[0].(map[string]interface{})
	var qry message.CreateHistArgs
	if err := mapstructure.Decode(qv, &qry); err != nil {
		return errors.Wrap(err, "failed#2 to decode qry map")
	}
	ref, err := sbot.ParseRef(qry.Id)
	if err != nil {
		return errors.Wrapf(err, "illegal ref: %s", qry.Id)
	}
	feedRef, ok := ref.(*sbot.FeedRef)
	if !ok {
		return errors.Wrapf(err, "illegal ref type: %s", qry.Id)
	}

	// check what we got
	gossipIdx := hist.Repo.GossipIndex()
	latestKey := librarian.Addr(fmt.Sprintf("latest:%s", ref.Ref()))
	latestObv, err := gossipIdx.Get(ctx, latestKey)
	if err != nil {
		return errors.Wrap(err, "failed to get latest")
	}
	latestV, err := latestObv.Value()
	if err != nil {
		return errors.Wrap(err, "failed to observ latest")
	}

	// act accordingly
	switch v := latestV.(type) {
	case librarian.UnsetValue: // don't have the feed - nothing to do?
		if hist.Promisc {
			err := gossipIdx.Set(ctx, latestKey, margaret.BaseSeq(0))
			if err != nil {
				return errors.Wrap(err, "failed to set unknown key")
			}
		}
	case margaret.BaseSeq:
		if qry.Seq >= v { // more than we got
			return errors.Wrap(req.Stream.Close(), "pour: failed to close")
		}
		// TODO: multilog
		seqs, err := hist.Repo.FeedSeqs(*feedRef)
		if err != nil {
			return errors.Wrap(err, "failed to get internal seqs")
		}
		rest := seqs[qry.Seq-1:]
		if len(rest) > 150 { // batch - slow but steady
			rest = rest[:150]
		}
		log := hist.Repo.Log()
		for i, rSeq := range rest {
			v, err := log.Get(rSeq)
			if err != nil {
				return errors.Wrapf(err, "load message %d", i)
			}
			stMsg := v.(message.StoredMessage)
			if err := req.Stream.Pour(ctx, message.RawSignedMessage{stMsg.Raw}); err != nil {
				return errors.Wrap(err, "failed to pour msg to remote peer")

			}
		}
		hist.Info.Log("sent", ref.Ref(), "from", qry.Seq, "rest", len(rest))
	}
	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}
