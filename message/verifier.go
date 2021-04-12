package message

import (
	"fmt"
	"sync"

	"go.cryptoscope.co/librarian"
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

func (vs *VerifySink) GetSink(ref refs.FeedRef) (SequencedSink, error) {
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

	var ms = MargaretSaver{vs.rxlog}
	snk = NewVerifySink(ref, msg, msg, ms, vs.hmacSec)
	vs.sinks[ref.Ref()] = snk
	return snk, nil
}

type MargaretSaver struct {
	margaret.Log
}

func (ms MargaretSaver) Save(msg refs.Message) error {
	_, err := ms.Log.Append(msg)
	return err
}

func firstMessage(author refs.FeedRef) refs.KeyValueRaw {
	return refs.KeyValueRaw{
		Value: refs.Value{
			Previous: nil,
			Author:   author,
			Sequence: 0,
		},
	}
}

func (vs VerifySink) getLatestMsg(ref refs.FeedRef) (refs.Message, error) {
	frAddr := storedrefs.Feed(ref)
	userLog, err := vs.feeds.Get(frAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to open sublog for user: %w", err)
	}
	latest, err := userLog.Seq().Value()
	if err != nil {
		return nil, fmt.Errorf("failed to observe latest: %w", err)
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
			return nil, fmt.Errorf("failed to look up root seq for latest user sublog: %w", err)
		}
		msgV, err := vs.rxlog.Get(rxVal.(margaret.Seq))
		if err != nil {
			return nil, fmt.Errorf("failed retreive stored message: %w", err)
		}

		var ok bool
		latestMsg, ok := msgV.(refs.Message)
		if !ok {
			return nil, fmt.Errorf("fetch: wrong message type. expected %T - got %T", latestMsg, msgV)
		}
		return latestMsg, nil

	default:
		return nil, fmt.Errorf("unexpected return value from index: %T", latest)
	}
	panic("unreadable")
}
