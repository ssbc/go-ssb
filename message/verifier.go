package message

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

// NewVerificationSinker supplies a sink per author that skip duplicate messages.
func NewVerificationSinker(rxlog margaret.Log, feeds multilog.MultiLog, hmacSec *[32]byte) (*VerifySink, error) {
	return &VerifySink{
		hmacSec: hmacSec,

		rxlog: rxlog,
		feeds: feeds,

		mu:    new(sync.Mutex),
		sinks: make(verifyFanIn),
	}, nil
}

type verifyFanIn map[string]SequencedSink

type VerifySink struct {
	rxlog margaret.Log
	feeds multilog.MultiLog

	hmacSec *[32]byte

	mu    *sync.Mutex
	sinks verifyFanIn
}

func (vs *VerifySink) GetSink(ref *refs.FeedRef) (SequencedSink, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// do we have an open sink for this feed already?
	snk, has := vs.sinks[ref.Ref()]
	if has {
		return snk, nil
	}

	msg, err := vs.getLatestMsg(ref)
	if err != nil {
		return nil, err
	}

	// TODO: get rid of all these empty interface wrappers
	storeSnk := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}

		_, err = vs.rxlog.Append(val)
		return errors.Wrap(err, "failed to append verified message to rootLog")
	})

	snk = NewVerifySink(ref, msg, msg, storeSnk, vs.hmacSec)
	vs.sinks[ref.Ref()] = snk
	return snk, nil
}

func firstMessage(r *refs.FeedRef) refs.KeyValueRaw {
	author := r.Copy()
	return refs.KeyValueRaw{
		Value: refs.Value{
			Previous: nil,
			Author:   *author,
			Sequence: margaret.BaseSeq(0),
		},
	}
}

func (vs VerifySink) getLatestMsg(ref *refs.FeedRef) (refs.Message, error) {
	frAddr := storedrefs.Feed(ref)
	userLog, err := vs.feeds.Get(frAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open sublog for user")
	}
	latest, err := userLog.Seq().Value()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to observe latest")
	}

	switch v := latest.(type) {
	case librarian.UnsetValue:
		// nothing stored, fetch from zero
		return firstMessage(ref), nil
	case margaret.BaseSeq:
		if v < 0 {
			return firstMessage(ref), nil
		}

		rxVal, err := userLog.Get(v)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to look up root seq for latest user sublog")
		}
		msgV, err := vs.rxlog.Get(rxVal.(margaret.Seq))
		if err != nil {
			return nil, errors.Wrapf(err, "failed retreive stored message")
		}

		var ok bool
		latestMsg, ok := msgV.(refs.Message)
		if !ok {
			return nil, errors.Errorf("fetch: wrong message type. expected %T - got %T", latestMsg, msgV)
		}
		return latestMsg, nil

	default:
		return nil, errors.Errorf("unexpected return value from index: %T", latest)
	}
	panic("unreadable")
}
