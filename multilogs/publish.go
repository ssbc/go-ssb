package multilogs

import (
	"log"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

type publishLog struct {
	margaret.Log
	rootLog margaret.Log
	key     ssb.KeyPair
	hmac    *[32]byte
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

	// TODO: user control, if they want to expose a timestamp would be cool

	newMsg.Timestamp = time.Now().UnixNano() / 1000000
	// or .Unix() * 1000 but needs to be ascending between calls
	// the requirement is supposed to be lifted but I don't want to depend on it _right now_
	// it's on my errata for shs2

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

	mr, signedMessage, err := newMsg.Sign(pl.key.Pair.Secret[:], pl.hmac)
	if err != nil {
		return nil, err
	}

	var stored message.StoredMessage
	stored.Timestamp = time.Now() // "rx"
	stored.Author = pl.key.Id
	stored.Previous = newMsg.Previous
	stored.Sequence = newMsg.Sequence
	stored.Key = mr
	stored.Raw = signedMessage

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
	return openPublish(rootLog, sublogs, kp)
}

func OpenPublishLogWithHMAC(rootLog margaret.Log, sublogs multilog.MultiLog, kp ssb.KeyPair, hmackey []byte) (margaret.Log, error) {
	pl, err := openPublish(rootLog, sublogs, kp)
	if n := len(hmackey); n != 32 {
		return nil, errors.Errorf("publish/hmac: wrong hmackey length (%d)", n)
	}
	var hmacSec [32]byte
	copy(hmacSec[:], hmackey)
	pl.hmac = &hmacSec
	return pl, err
}

func openPublish(rootLog margaret.Log, sublogs multilog.MultiLog, kp ssb.KeyPair) (*publishLog, error) {
	if sublogs == nil {
		return nil, errors.Errorf("no sublog for publish")
	}
	authorLog, err := sublogs.Get(librarian.Addr(kp.Id.ID))
	if err != nil {
		return nil, errors.Wrap(err, "publish: failed to open sublog for author")
	}

	return &publishLog{
		Log:     authorLog,
		rootLog: rootLog,
		key:     kp,
	}, nil
}
