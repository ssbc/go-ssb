// SPDX-License-Identifier: MIT

package message

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message/legacy"
	gabbygrove "go.mindeco.de/ssb-gabbygrove"
	refs "go.mindeco.de/ssb-refs"
)

type publishLog struct {
	sync.Mutex
	margaret.Log
	rootLog margaret.Log

	create creater
}

func (p *publishLog) Publish(content interface{}) (refs.MessageRef, error) {
	seq, err := p.Append(content)
	if err != nil {
		return refs.MessageRef{}, err
	}

	val, err := p.rootLog.Get(seq)
	if err != nil {
		return refs.MessageRef{}, fmt.Errorf("publish: failed to get new stored message: %w", err)
	}

	kv, ok := val.(refs.Message)
	if !ok {
		return refs.MessageRef{}, fmt.Errorf("publish: unsupported keyer %T", val)
	}

	return kv.Key(), nil
}

// Get retreives the message object by traversing the authors sublog to the root log
func (pl publishLog) Get(s margaret.Seq) (interface{}, error) {

	idxv, err := pl.Log.Get(s)
	if err != nil {
		return nil, fmt.Errorf("publish get: failed to retreive sequence for the root log: %w", err)
	}

	msgv, err := pl.rootLog.Get(idxv.(margaret.Seq))
	if err != nil {
		return nil, fmt.Errorf("publish get: failed to retreive message from rootlog: %w", err)
	}
	return msgv, nil
}

/*
TODO: do the same for Query()? but how?

=> just overwrite publish on the authorLog for now
*/

func (pl *publishLog) Append(val interface{}) (margaret.Seq, error) {
	pl.Lock()
	defer pl.Unlock()

	// current state of the local sig-chain
	var (
		nextPrevious refs.MessageRef
		nextSequence = int64(-1)
	)

	currSeq, err := pl.Seq().Value()
	if err != nil {
		return nil, fmt.Errorf("publishLog: failed to establish current seq: %w", err)
	}
	seq, ok := currSeq.(margaret.Seq)
	if !ok {
		return nil, fmt.Errorf("publishLog: invalid sequence from publish sublog %v: %T", currSeq, currSeq)
	}

	currRootSeq, err := pl.Log.Get(seq)
	if err != nil && !luigi.IsEOS(err) {
		return nil, fmt.Errorf("publishLog: failed to retreive current msg: %w", err)
	}
	if luigi.IsEOS(err) { // new feed
		nextSequence = 1
	} else {
		currMM, err := pl.rootLog.Get(currRootSeq.(margaret.Seq))
		if err != nil {
			return nil, fmt.Errorf("publishLog: failed to establish current seq: %w", err)
		}
		mm, ok := currMM.(refs.Message)
		if !ok {
			return nil, fmt.Errorf("publishLog: invalid value at sequence %v: %T", currSeq, currMM)
		}
		nextPrevious = mm.Key()
		nextSequence = mm.Seq() + 1
	}

	nextMsg, err := pl.create.Create(val, nextPrevious, nextSequence)
	if err != nil {
		return nil, fmt.Errorf("failed to create next msg: %w", err)
	}

	rlSeq, err := pl.rootLog.Append(nextMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to append new msg: %w", err)
	}

	return rlSeq, nil
}

// OpenPublishLog needs the base datastore (root or receive log - offset2)
// and the userfeeds with all the sublog and uses the passed keypair to find the corresponding user feed
// the returned log's append function is then used to create new messages.
// these messages are constructed in the legacy SSB way: The poured object is JSON v8-like pretty printed and then NaCL signed,
// then it's pretty printed again (now with the signature inside the message) to construct it's SHA256 hash,
// which is used to reference it (by replys and it's previous)
func OpenPublishLog(rootLog margaret.Log, sublogs multilog.MultiLog, kp ssb.KeyPair, opts ...PublishOption) (ssb.Publisher, error) {
	authorLog, err := sublogs.Get(storedrefs.Feed(kp.Id))
	if err != nil {
		return nil, fmt.Errorf("publish: failed to open sublog for author: %w", err)
	}

	pl := &publishLog{
		Log:     authorLog,
		rootLog: rootLog,
	}

	switch kp.Id.Algo() {
	case refs.RefAlgoFeedSSB1:
		pl.create = &legacyCreate{
			key: kp,
		}
	case refs.RefAlgoFeedGabby:
		pl.create = &gabbyCreate{
			enc: gabbygrove.NewEncoder(kp.Pair.Secret),
		}
	default:
		return nil, fmt.Errorf("publish: unsupported feed algorithm: %s", kp.Id.Algo())
	}

	for i, o := range opts {
		if err := o(pl); err != nil {
			return nil, fmt.Errorf("publish: option %d failed: %w", i, err)
		}
	}

	return pl, nil
}

type PublishOption func(*publishLog) error

func SetHMACKey(hmackey *[32]byte) PublishOption {
	return func(pl *publishLog) error {
		switch cv := pl.create.(type) {
		case *legacyCreate:
			cv.hmac = hmackey
		case *gabbyCreate:
			cv.enc.WithHMAC(hmackey[:])
		default:
			return fmt.Errorf("hmac: unknown creater: %T", cv)
		}
		return nil
	}
}

func UseNowTimestamps(yes bool) PublishOption {
	return func(pl *publishLog) error {
		switch cv := pl.create.(type) {
		case *legacyCreate:
			cv.setTimestamp = yes

		case *gabbyCreate:
			cv.enc.WithNowTimestamps(yes)

		default:
			return fmt.Errorf("setTimestamp: unknown creater: %T", cv)
		}
		return nil
	}
}

type creater interface {
	Create(val interface{}, prev refs.MessageRef, seq int64) (refs.Message, error)
}

type legacyCreate struct {
	key          ssb.KeyPair
	hmac         *[32]byte
	setTimestamp bool
}

func (lc legacyCreate) Create(val interface{}, prev refs.MessageRef, seq int64) (refs.Message, error) {
	// prepare persisted message
	var stored legacy.StoredMessage
	stored.Timestamp_ = time.Now() // "rx"
	stored.Author_ = lc.key.Id

	// set metadata
	var newMsg legacy.LegacyMessage
	newMsg.Hash = "sha256"
	newMsg.Author = lc.key.Id.Ref()
	if seq > 1 {
		newMsg.Previous = &prev
	}
	newMsg.Sequence = margaret.BaseSeq(seq)

	if bindata, ok := val.([]byte); ok {
		bindata = bytes.TrimPrefix(bindata, []byte("box1:"))
		newMsg.Content = base64.StdEncoding.EncodeToString(bindata) + ".box"
	} else {
		newMsg.Content = val
	}

	if lc.setTimestamp {
		newMsg.Timestamp = time.Now().UnixNano() / 1000000
	}

	mr, signedMessage, err := newMsg.Sign(lc.key.Pair.Secret[:], lc.hmac)
	if err != nil {
		return nil, err
	}

	stored.Previous_ = newMsg.Previous
	stored.Sequence_ = newMsg.Sequence
	stored.Key_ = mr
	stored.Raw_ = signedMessage
	return &stored, nil
}

type gabbyCreate struct {
	enc *gabbygrove.Encoder
}

func (pc gabbyCreate) Create(val interface{}, prev refs.MessageRef, seq int64) (refs.Message, error) {
	br, err := gabbygrove.NewBinaryRef(prev)
	if err != nil {
		return nil, err
	}
	nextSeq := uint64(seq)
	tr, _, err := pc.enc.Encode(nextSeq, br, val)
	if err != nil {
		return nil, fmt.Errorf("gabby: failed to encode content: %w", err)
	}
	return tr, nil
}
