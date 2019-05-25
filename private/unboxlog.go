package private

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/legacy"
)

type unboxedLog struct {
	root, seqlog margaret.Log
	kp           *ssb.KeyPair
}

// NewUnboxerLog expects the sequence numbers, that are returned from seqlog, to be decryptable by kp.
func NewUnboxerLog(root, seqlog margaret.Log, kp *ssb.KeyPair) margaret.Log {
	il := unboxedLog{
		root:   root,
		seqlog: seqlog,
		kp:     kp,
	}
	return il
}

func (il unboxedLog) Seq() luigi.Observable {
	return il.seqlog.Seq()
}

func (il unboxedLog) Get(seq margaret.Seq) (interface{}, error) {
	return nil, errors.Errorf("TODO: unbox here too?")

	v, err := il.seqlog.Get(seq)
	if err != nil {
		return nil, errors.Wrap(err, "seqlog: 1st lookup failed")
	}

	rv, err := il.root.Get(v.(margaret.Seq))
	return rv, errors.Wrap(err, "seqlog: root lookup failed")
}

// Query maps the sequence values in seqlog to an unboxed version of the message
func (il unboxedLog) Query(args ...margaret.QuerySpec) (luigi.Source, error) {
	src, err := il.seqlog.Query(args...)
	if err != nil {
		return nil, errors.Wrap(err, "unboxLog: error querying seqlog")
	}

	return mfr.SourceMap(src, func(ctx context.Context, iv interface{}) (interface{}, error) {
		val, err := il.root.Get(iv.(margaret.Seq))
		if err != nil {
			return nil, errors.Wrapf(err, "unboxLog: error getting v(%v) from seqlog log", iv)
		}

		amsg, ok := val.(ssb.Message)
		if !ok {
			return nil, errors.Errorf("wrong message type. expected %T - got %T", amsg, val)
		}

		clearContent, err := Unbox(il.kp, string(amsg.ContentBytes()))
		if err != nil {
			return nil, errors.Wrap(err, "unboxLog: unbox failed")
		}

		// re-wrap the unboxed in the original
		var contentVal map[string]interface{}
		err = json.Unmarshal(clearContent, &contentVal)
		if err != nil {
			return nil, errors.Wrap(err, "unboxLog: failed to make contentVal")
		}

		author := amsg.Author()
		// TODO: fill all the fields

		// map unboxed content int something that looks like value
		var unboxedMsg legacy.SignedLegacyMessage
		// unboxedMsg.Previous = amsg.Previous()
		unboxedMsg.Author = author.Ref()
		// unboxedMsg.Sequence = msg.Sequence
		// unboxedMsg.Timestamp = msg.Timestamp.UnixNano() / 100000
		unboxedMsg.Hash = "go-ssb-unboxed"
		unboxedMsg.Content = contentVal
		unboxedMsg.Signature = "go-ssb-unboxed"

		var msg ssb.Value
		msg.Author = *author
		msg.Content, err = json.Marshal(unboxedMsg)
		return msg, errors.Wrap(err, "unboxLog: failed to encode unboxed msg")
	}), nil
}

// Append doesn't work on this log. They need to go through the proper channels.
func (il unboxedLog) Append(interface{}) (margaret.Seq, error) {
	return nil, errors.New("can't append to seqloged log, sorry")
}
