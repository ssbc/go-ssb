package sbot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

var _ ssb.Replicator = (*Sbot)(nil)

type graphReplicator struct {
	bot     *Sbot
	current *lister
}

func (s *Sbot) newGraphReplicator() (*graphReplicator, error) {
	var r graphReplicator
	r.bot = s
	r.current = newLister()

	replicateEvt := log.With(s.info, "event", "update-replicate")
	update := r.makeUpdater(replicateEvt, s.KeyPair.Id, int(s.hopCount))

	// update for new messages but only every 15seconds
	go debounce(s.rootCtx, 3*time.Second, s.ReceiveLog.Seq(), update)

	return &r, nil
}

// makeUpdater returns a func that does the hop-walk and block checks, used together with debounce
func (r *graphReplicator) makeUpdater(log log.Logger, self refs.FeedRef, hopCount int) func() {
	return func() {
		start := time.Now()
		newWants := r.bot.GraphBuilder.Hops(self, hopCount)
		level.Debug(log).Log("feed-want-count", newWants.Count(), "hops", hopCount, "took", time.Since(start))

		refs, err := newWants.List()
		if err != nil {
			level.Error(log).Log("msg", "want list failed", "err", err, "wants", newWants.Count())
			return
		}
		for _, ref := range refs {
			r.current.feedWants.AddRef(ref)
		}

		// make sure we dont fetch and allow blocked feeds
		g, err := r.bot.GraphBuilder.Build()
		if err != nil {
			level.Error(log).Log("msg", "failed to build blocks", "err", err)
			return
		}

		newBlocked := g.BlockedList(self)
		lst, err := newBlocked.List()
		if err == nil {
			for _, bf := range lst {
				r.current.blocked.AddRef(bf)
				r.current.feedWants.Delete(bf)
			}
		}
	}
}

func debounce(ctx context.Context, interval time.Duration, obs luigi.Observable, work func()) {
	var seqMu sync.Mutex
	var seq = margaret.SeqEmpty
	timer := time.NewTimer(interval)

	handle := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			return err
		}
		newSeq, ok := val.(margaret.BaseSeq)
		if !ok {
			return fmt.Errorf("graph rebuild debounce: wrong type: %T", val)
		}
		seqMu.Lock()
		seq = newSeq
		timer.Reset(interval)
		seqMu.Unlock()
		return nil
	})
	done := obs.Register(handle)

	for {
		select {
		case <-ctx.Done():
			done()
			return

		case <-timer.C:
			seqMu.Lock()
			if seq != margaret.SeqEmpty {
				work()
				seq = margaret.SeqEmpty
			}
			seqMu.Unlock()
		}
	}
}

func (r *graphReplicator) Block(ref refs.FeedRef)   { r.current.blocked.AddRef(ref) }
func (r *graphReplicator) Unblock(ref refs.FeedRef) { r.current.blocked.Delete(ref) }

func (r *graphReplicator) Replicate(ref refs.FeedRef)     { r.current.feedWants.AddRef(ref) }
func (r *graphReplicator) DontReplicate(ref refs.FeedRef) { r.current.feedWants.Delete(ref) }

func (r *graphReplicator) Lister() ssb.ReplicationLister { return r.current }

type lister struct {
	feedWants *ssb.StrFeedSet
	blocked   *ssb.StrFeedSet
}

func newLister() *lister {
	return &lister{
		feedWants: ssb.NewFeedSet(0),
		blocked:   ssb.NewFeedSet(0),
	}
}

func (l lister) Authorize(remote refs.FeedRef) error {
	if l.blocked.Has(remote) {
		return errors.New("peer blocked")
	}

	if l.feedWants.Has(remote) {
		return nil
	}
	return errors.New("nope - access denied")
}

func (l lister) ReplicationList() *ssb.StrFeedSet { return l.feedWants }
func (l lister) BlockList() *ssb.StrFeedSet       { return l.blocked }
