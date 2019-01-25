package multilogs

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"golang.org/x/crypto/ed25519"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

type publishLog struct {
	margaret.Log
	rootLog margaret.Log
	key     ssb.KeyPair
}

/* Get retreives the message object by traversing the authors sublog to the root log
func (pl publishLog) Get(s margaret.Seq) (interface{}, error) {

	idxv, err := pl.authorLog.Get(s)
	if err != nil {
		return nil, errors.Wrap(err, "publish get: failed to retreive sequence for the root log")
	}

	msgv, err := pl.rootLog.Get(idxv.(margaret.Seq))
	if err != nil {
		return nil, errors.Wrap(err, "publish get: failed to retreive message from rootlog")
	}
	return msgv, nil
}

TODO: do the same for Query()? but how?

=> just overwrite publish on the authorLog for now
*/

func (pl publishLog) Append(val interface{}) (margaret.Seq, error) {

	// set metadata
	var newMsg message.LegacyMessage
	// user control would be nice here
	newMsg.Timestamp = time.Now().Unix()
	newMsg.Author = pl.key.Id.Ref()
	newMsg.Hash = "sha256"
	newMsg.Content = val

	// current state of the local sig-chain
	currSeq, err := pl.Seq().Value()
	if err != nil {
		return nil, errors.Wrap(err, "publishLog: failed to establish current seq")
	}
	seq := currSeq.(margaret.Seq)
	currRootSeq, err := pl.Get(seq)
	if err != nil && !luigi.IsEOS(err) {
		return nil, errors.Wrap(err, "publishLog: failed to retreive current msg")
	}
	if luigi.IsEOS(err) { // new feed
		newMsg.Previous = nil
		newMsg.Sequence = 1
	} else {
		currV, err := pl.rootLog.Get(currRootSeq.(margaret.Seq))
		if err != nil {
			return nil, errors.Wrap(err, "publishLog: failed to establish current seq")
		}

		currMsg, ok := currV.(message.StoredMessage)
		if !ok {
			return nil, errors.Errorf("publishLog: invalid value at sequence %v: %T", currSeq, currV)
		}

		newMsg.Previous = currMsg.Key
		newMsg.Sequence = margaret.BaseSeq(currMsg.Sequence + 1)
	}

	// flatten interface{} content value
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(newMsg); err != nil {
		return nil, errors.Wrap(err, "publishLog: failed to encode msg")
	}

	// pretty-print v8-like
	pp, err := message.EncodePreserveOrder(buf.Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "publishLog: failed to encode legacy msg")
	}

	sig := ed25519.Sign(pl.key.Pair.Secret[:], pp)

	var signedMsg message.SignedLegacyMessage
	signedMsg.LegacyMessage = newMsg
	signedMsg.Signature = message.EncodeSignature(sig)

	// encode again, now with the signature to get the hash of the message
	buf.Reset()
	if err := json.NewEncoder(&buf).Encode(signedMsg); err != nil {
		return nil, errors.Wrap(err, "publishLog: failed to encode new signed msg")
	}
	ppWithSig, err := message.EncodePreserveOrder(buf.Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "publishLog: failed to encode legacy msg")
	}
	v8warp, err := message.InternalV8Binary(ppWithSig)
	if err != nil {
		return nil, errors.Wrapf(err, "publishLog: ssb Sign(%s:%d): could not v8 escape message", pl.key.Id.Ref(), signedMsg.Sequence)
	}

	h := sha256.New()
	io.Copy(h, bytes.NewReader(v8warp))

	mr := ssb.MessageRef{
		Hash: h.Sum(nil),
		Algo: ssb.RefAlgoSHA256,
	}

	var stored message.StoredMessage
	stored.Author = pl.key.Id
	stored.Previous = signedMsg.Previous
	stored.Key = &mr
	stored.Timestamp = time.Now() // "rx"
	stored.Sequence = newMsg.Sequence
	stored.Raw = buf.Bytes()

	_, err = pl.rootLog.Append(stored)
	if err != nil {
		return nil, errors.Wrap(err, "failed to append new msg")
	}

	log.Println("new message key:", mr.Ref())
	return newMsg.Sequence - 1, nil
}

// OpenPublishLog needs the base datastore (root or receive log - offset2)
// and the userfeeds with all the sublog and uses the passed keypair to find the corresponding user feed
// the returned sink is then used to create new messages.
// warning: it is assumed that the
// these messages are constructed in the legacy SSB way: The poured object is JSON v8-like pretty printed and then NaCL signed,
// then it's pretty printed again (now with the signature inside the message) to construct it's SHA256 hash,
// which is used to reference it (by replys and it's previous)
func OpenPublishLog(rootLog margaret.Log, sublogs multilog.MultiLog, kp ssb.KeyPair) (margaret.Log, error) {
	if sublogs == nil {
		return nil, errors.Errorf("no sublog for publish")
	}
	authorLog, err := sublogs.Get(librarian.Addr(kp.Id.ID))
	if err != nil {
		return nil, errors.Wrap(err, "publish: failed to open sublog for author")
	}

	return publishLog{
		Log:     authorLog,
		rootLog: rootLog,
		key:     kp,
	}, nil
}
