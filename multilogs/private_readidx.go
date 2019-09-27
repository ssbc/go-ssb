package multilogs

import (
	"bytes"
	"context"
	"encoding/base64"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	gabbygrove "go.mindeco.de/ssb-gabbygrove"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
)

const IndexNamePrivates = "privates"

// not strictly a multilog but allows multiple keys and gives us the good resumption
func OpenPrivateRead(log kitlog.Logger, r repo.Interface, kp *ssb.KeyPair) (multilog.MultiLog, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, IndexNamePrivates, func(ctx context.Context, seq margaret.Seq, val interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := val.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}

		msg, ok := val.(ssb.Message)
		if !ok {
			err := errors.Errorf("private/readidx: error casting message. got type %T", val)
			return err
		}

		var boxedContent []byte
		switch msg.Author().Algo {
		case ssb.RefAlgoFeedSSB1:
			input := msg.ContentBytes()
			if !(input[0] == '"' && input[len(input)-1] == '"') {
				return nil // not a json string
			}
			b64data := bytes.TrimSuffix(input[1:], []byte(".box\""))
			boxedData := make([]byte, len(b64data))

			n, err := base64.StdEncoding.Decode(boxedData, b64data)
			if err != nil {
				err = errors.Wrap(err, "private/readidx: invalid b64 encoding")
				log.Log("event", "debug", "msg", "unboxLog b64 decode failed", "err", err)
				return nil
			}
			boxedContent = boxedData[:n]

		case ssb.RefAlgoFeedGabby:
			mm, ok := val.(multimsg.MultiMessage)
			if !ok {
				mmPtr, ok := val.(*multimsg.MultiMessage)
				if !ok {
					err := errors.Errorf("private/readidx: error casting message. got type %T", val)
					return err
				}
				mm = *mmPtr
			}
			tr, ok := mm.AsGabby()
			if !ok {
				err := errors.Errorf("private/readidx: error getting gabby msg")
				return err
			}
			evt, err := tr.UnmarshaledEvent()
			if err != nil {
				return errors.Wrap(err, "private/readidx: error unpacking event from stored message")
			}
			if evt.Content.Type != gabbygrove.ContentTypeArbitrary {
				return nil
			}
			boxedContent = bytes.TrimPrefix(tr.Content, []byte("box1:"))
		default:
			err := errors.Errorf("private/readidx: unknown feed type: %s", kp.Id.Algo)
			log.Log("event", "error", "msg", "unahndled type", "err", err)
			return err
		}

		if _, err := private.Unbox(kp, boxedContent); err != nil {
			return nil
		}

		userPrivs, err := mlog.Get(kp.Id.StoredAddr())
		if err != nil {
			return errors.Wrap(err, "private/readidx: error opening priv sublog")
		}

		_, err = userPrivs.Append(seq.Seq())
		return errors.Wrapf(err, "private/readidx: error appending PM for %s", kp.Id.Ref())
	})
}
