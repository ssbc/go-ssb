package multilogs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"time"

	"golang.org/x/crypto/ed25519"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

func OpenPublishLog(rootLog, authorLog margaret.Log, kp ssb.KeyPair) (luigi.Sink, error) {
	snk := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			// sink closed? - chance for cleanup
			return err
		}

		// set metadata
		var newMsg message.LegacyMessage
		// user control would be nice here
		// newMsg.Timestamp = time.Now().Unix()
		newMsg.Author = kp.Id.Ref()
		newMsg.Hash = "sha256"
		newMsg.Content = val

		// current state of the local sig-chain
		currSeq, err := authorLog.Seq().Value()
		if err != nil {
			return errors.Wrap(err, "failed to establish current seq")
		}
		seq := currSeq.(margaret.Seq)
		currRootSeq, err := authorLog.Get(seq)
		if err != nil && !luigi.IsEOS(err) {
			return errors.Wrap(err, "failed to retreive current msg")
		}
		if luigi.IsEOS(err) { // new feed
			newMsg.Previous = nil
			newMsg.Sequence = 1
		} else {
			currV, err := rootLog.Get(currRootSeq.(margaret.Seq))
			if err != nil {
				return errors.Wrap(err, "failed to establish current seq")
			}

			currMsg, ok := currV.(message.StoredMessage)
			if !ok {
				return errors.Errorf("publish log: invalid value at sequence %v: %T", currSeq, currV)
			}

			newMsg.Previous = currMsg.Key
			newMsg.Sequence = margaret.BaseSeq(currMsg.Sequence + 1)
		}

		// flatten interface{} content value
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(newMsg); err != nil {
			return errors.Wrap(err, "failed to encode msg")
		}

		// pretty-print v8-like
		pp, err := message.EncodePreserveOrder(buf.Bytes())
		if err != nil {
			return errors.Wrap(err, "failed to encode legacy msg")
		}

		sig := ed25519.Sign(kp.Pair.Secret[:], pp)

		var signedMsg message.SignedLegacyMessage
		signedMsg.LegacyMessage = newMsg
		signedMsg.Signature = message.EncodeSignature(sig)

		// encode again, now with the signature to get the hash of the message
		buf.Reset()
		if err := json.NewEncoder(&buf).Encode(signedMsg); err != nil {
			return errors.Wrap(err, "failed to encode new signed msg")
		}
		ppWithSig, err := message.EncodePreserveOrder(buf.Bytes())
		if err != nil {
			return errors.Wrap(err, "failed to encode legacy msg")
		}
		v8warp, err := message.InternalV8Binary(ppWithSig)
		if err != nil {
			return errors.Wrapf(err, "ssb Sign(%s:%d): could not v8 escape message", kp.Id.Ref(), signedMsg.Sequence)
		}

		h := sha256.New()
		io.Copy(h, bytes.NewReader(v8warp))

		mr := ssb.MessageRef{
			Hash: h.Sum(nil),
			Algo: ssb.RefAlgoSHA256,
		}

		var stored message.StoredMessage
		stored.Author = kp.Id
		stored.Previous = signedMsg.Previous
		stored.Key = &mr
		stored.Timestamp = time.Now() // "rx"
		stored.Sequence = newMsg.Sequence
		stored.Raw = buf.Bytes()

		if _, err := rootLog.Append(stored); err != nil {
			return errors.Wrap(err, "failed to append new msg")
		}

		// log.Printf("published: %v\n%s", nextSeq.Seq(), string(ppWithSig))
		return err
	})

	return snk, nil
}
