package gossip

import (
	"context"
	"encoding/json"
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
	qv := req.Args[0].(map[string]interface{})
	var qry message.CreateHistArgs
	if err := mapstructure.Decode(qv, &qry); err != nil {
		return errors.Wrap(err, "failed#2 to decode qry map")
	}

	ref, err := sbot.ParseRef(qry.Id)
	if err != nil {
		return errors.Wrapf(err, "illegal ref: %s", qry.Id)
	}

	latestObv, err := hist.Repo.GossipIndex().Get(ctx, librarian.Addr(fmt.Sprintf("latest:%s", ref.Ref())))
	if err != nil {
		return errors.Wrap(err, "failed to get latest")
	}
	latestV, err := latestObv.Value()
	if err != nil {
		return errors.Wrap(err, "failed to observ latest")
	}

	switch v := latestV.(type) {
	case librarian.UnsetValue:
	case margaret.Seq:
		if qry.Seq >= v {
			return errors.Wrap(req.Stream.Close(), "pour: failed to close")
		}

		fr := ref.(*sbot.FeedRef)
		seqs, err := hist.Repo.FeedSeqs(*fr)
		if err != nil {
			return errors.Wrap(err, "failed to get internal seqs")
		}
		rest := seqs[qry.Seq-1:]
		if len(rest) > 500 { // batch - slow but steady
			rest = rest[:150]
		}
		log := hist.Repo.Log()
		for i, rSeq := range rest {
			v, err := log.Get(rSeq)
			if err != nil {
				return errors.Wrapf(err, "load message %d", i)
			}
			stMsg := v.(message.StoredMessage)

			type fuck struct{ json.RawMessage }
			if err := req.Stream.Pour(ctx, fuck{stMsg.Raw}); err != nil {
				return errors.Wrap(err, "failed to pour msg to remote peer")

			}
		}
		hist.Info.Log("sent", ref.Ref(), "from", qry.Seq, "rest", len(rest))
	}
	return errors.Wrap(req.Stream.Close(), "pour: failed to close")
}
