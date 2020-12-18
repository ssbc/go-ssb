// SPDX-License-Identifier: MIT

// Package message contains abstract verification and publish helpers.
package message

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	gabbygrove "go.mindeco.de/ssb-gabbygrove"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/message/legacy"
)

type SequencedSink interface {
	margaret.Seq
	luigi.Sink // TODO: de-luigi-fy
}

// NewVerifySink returns a sink that does message verification and appends corret messages to the passed log.
// it has to be used on a feed by feed bases, the feed format is decided by the passed feed reference.
// TODO: start and abs could be the same parameter
// TODO: needs configuration for hmac and what not..
// => maybe construct those from a (global) ref register where all the suffixes live with their corresponding network configuration?
func NewVerifySink(who *refs.FeedRef, start margaret.Seq, abs refs.Message, snk luigi.Sink, hmacKey *[32]byte) SequencedSink {
	sd := &streamDrain{
		who:       who,
		latestSeq: margaret.BaseSeq(start.Seq()),
		latestMsg: abs,
		storage:   snk,
	}
	switch who.Algo {
	case refs.RefAlgoFeedSSB1:
		sd.verify = legacyVerify{hmacKey: hmacKey}
	case refs.RefAlgoFeedGabby:
		sd.verify = gabbyVerify{hmacKey: hmacKey}
	}
	return sd
}

type verifier interface {
	Verify([]byte) (refs.Message, error)
}

type legacyVerify struct {
	hmacKey *[32]byte
}

func (lv legacyVerify) Verify(rmsg []byte) (refs.Message, error) {
	ref, dmsg, err := legacy.Verify(rmsg, lv.hmacKey)
	if err != nil {
		return nil, err
	}

	return &legacy.StoredMessage{
		Author_:    &dmsg.Author,
		Previous_:  dmsg.Previous,
		Key_:       ref,
		Sequence_:  dmsg.Sequence,
		Timestamp_: time.Now(),
		Raw_:       rmsg,
	}, nil
}

type gabbyVerify struct {
	hmacKey *[32]byte
}

func (gv gabbyVerify) Verify(trBytes []byte) (msg refs.Message, err error) {
	var tr gabbygrove.Transfer
	if uErr := tr.UnmarshalCBOR(trBytes); uErr != nil {
		err = errors.Wrapf(uErr, "gabbyVerify: transfer unmarshal failed")
		return
	}

	defer func() {
		// TODO: change cbor encoder in gg
		if r := recover(); r != nil {
			if panicErr, ok := r.(error); ok {
				err = errors.Wrap(panicErr, "gabbyVerify: recovered from panic")
			} else {
				panic(r)
			}
		}
	}()
	if !tr.Verify(gv.hmacKey) {
		return nil, errors.Errorf("gabbyVerify: transfer verify failed")
	}
	msg = &tr
	return
}

type streamDrain struct {
	// gets the input from the screen and returns the next decoded message, if it is valid
	verify verifier

	who *refs.FeedRef // which feed is pulled

	// holds onto the current/newest method (for sequence check and prev hash compare)
	mu        sync.Mutex
	latestSeq margaret.BaseSeq
	latestMsg refs.Message

	storage luigi.Sink // TODO: change to margaret.Log
}

func (ld *streamDrain) Seq() int64 {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	return int64(ld.latestSeq)
}

// TODO: de-luigify
func (ld *streamDrain) Pour(ctx context.Context, v interface{}) error {
	ld.mu.Lock()
	defer ld.mu.Unlock()

	var msg []byte

	switch tv := v.(type) {
	case json.RawMessage:
		msg = tv

	case []uint8:
		msg = tv

	default:
		return errors.Errorf("verify: expected %T - got %T", msg, v)
	}

	next, err := ld.verify.Verify(msg)
	if err != nil {
		return errors.Wrapf(err, "message(%s:%d) verify failed", ld.who.ShortRef(), ld.latestSeq.Seq())
	}

	err = ValidateNext(ld.latestMsg, next)
	if err != nil {
		if err == errSkip {
			return nil
		}
		return err
	}

	err = ld.storage.Pour(ctx, next)
	if err != nil {
		return errors.Wrapf(err, "message(%s): failed to append message(%s:%d)", ld.who.ShortRef(), next.Key().Ref(), next.Seq())
	}

	ld.latestSeq = margaret.BaseSeq(next.Seq())
	ld.latestMsg = next
	return nil
}

func (ld streamDrain) Close() error { return nil } //ld.storage.Close() }

var errSkip = errors.New("ValidateNext: already got message")

// ValidateNext checks the author stays the same across the feed,
// that he previous hash is correct and that the sequence number is increasing correctly
// TODO: move all the message's publish and drains to it's own package
func ValidateNext(current, next refs.Message) error {
	nextSeq := next.Seq()

	if current == nil || current.Seq() == 0 {
		if nextSeq != 1 {
			return errors.Errorf("ValidateNext(%s:%d): first message has to have sequence 1, got %d", next.Author().ShortRef(), 0, nextSeq)
		}
		return nil
	}
	currSeq := current.Seq()

	author := current.Author()
	if !author.Equal(next.Author()) {
		return errors.Errorf("ValidateNext(%s:%d): wrong author: %s", author.ShortRef(), current.Seq(), next.Author().ShortRef())
	}

	if currSeq+1 != nextSeq {
		shouldSkip := next.Seq() > currSeq+1
		if shouldSkip {
			return errSkip
		}
		return errors.Errorf("ValidateNext(%s:%d): next.seq(%d) != curr.seq+1", author.ShortRef(), currSeq, nextSeq)
	}

	currKey := current.Key()
	if !currKey.Equal(next.Previous()) {
		return errors.Errorf("ValidateNext(%s:%d): previous compare failed expected:%s incoming:%v",
			author.Ref(),
			currSeq,
			current.Key().Ref(),
			next.Previous(),
		)
	}

	return nil
}
