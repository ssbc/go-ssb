package multilogs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/keks/persist"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
	gabbygrove "go.mindeco.de/ssb-gabbygrove"
	refs "go.mindeco.de/ssb-refs"
)

// NewCombinedIndex creates one big index which updates the multilogs users, byType, private and tangles.
// Compared to the "old" fatbot approach of just having 4 independant indexes,
// this one updates all 4 of them, resulting in less read-overhead
// while also being able to index private massages by tangle and type.
func NewCombinedIndex(repoPath string, box *private.Manager, self *refs.FeedRef, res *repo.SequenceResolver, u, p, bt, tan *roaring.MultiLog) (*combinedIndex, error) {
	r := repo.New(repoPath)
	statePath := r.GetPath(repo.PrefixMultiLog, "combined-state.json")
	mode := os.O_RDWR | os.O_EXCL
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		mode |= os.O_CREATE
	}
	os.MkdirAll(filepath.Dir(statePath), 0700)
	idxStateFile, err := os.OpenFile(statePath, mode, 0700)
	if err != nil {
		return nil, errors.Wrap(err, "error opening state file")
	}

	idx := &combinedIndex{
		self:  self,
		boxer: box,

		users:   u,
		private: p,
		byType:  bt,
		tangles: tan,

		seqresolver: res,

		file: idxStateFile,
		l:    &sync.Mutex{},
	}
	return idx, nil
}

var _ librarian.SinkIndex = (*combinedIndex)(nil)

type combinedIndex struct {
	self  *refs.FeedRef
	boxer *private.Manager

	users   *roaring.MultiLog
	private *roaring.MultiLog
	byType  *roaring.MultiLog
	tangles *roaring.MultiLog

	seqresolver *repo.SequenceResolver

	file *os.File
	l    *sync.Mutex
}

// Pour calls the processing function to add a value to a sublog.
func (slog *combinedIndex) Pour(ctx context.Context, swv interface{}) error {
	slog.l.Lock()
	defer slog.l.Unlock()

	sw, ok := swv.(margaret.SeqWrapper)
	if !ok {
		return errors.Errorf("error casting seq wrapper. got type %T", swv)
	}
	seq := sw.Seq()

	// todo: defer state save!?
	err := persist.Save(slog.file, seq)
	if err != nil {
		return errors.Wrap(err, "error saving current sequence number")
	}

	v := sw.Value()

	if isNulled, ok := v.(error); ok {
		if margaret.IsErrNulled(isNulled) {
			err = slog.seqresolver.Append(seq.Seq(), 0, time.Now(), time.Now())
			if err != nil {
				return errors.Wrap(err, "error updating sequence resolver (nulled message)")
			}
			return nil
		}
		return isNulled
	}

	abstractMsg, ok := v.(refs.Message)
	if !ok {
		return errors.Errorf("error casting message. got type %T", v)
	}

	err = slog.seqresolver.Append(seq.Seq(), abstractMsg.Seq(), abstractMsg.Claimed(), abstractMsg.Received())
	if err != nil {
		return errors.Wrap(err, "error updating sequence resolver")
	}

	author := abstractMsg.Author()

	authorLog, err := slog.users.Get(storedrefs.Feed(author))
	if err != nil {
		return errors.Wrap(err, "error opening sublog")
	}
	_, err = authorLog.Append(seq.Seq())
	if err != nil {
		return errors.Wrap(err, "error updating author sublog")
	}

	// decrypt box 1 & 2
	content := abstractMsg.ContentBytes()
	// TODO: gabby grove
	if content[0] != '{' { // assuming all other content is json objects
		cleartext, err := slog.tryDecrypt(abstractMsg, seq)
		if err != nil {
			if err == errSkip {
				return nil
			}
			return err
		}
		content = cleartext
	}

	// by type:...
	var jsonContent struct {
		Type    string
		Root    *refs.MessageRef
		Tangles refs.Tangles
	}

	err = json.Unmarshal(content, &jsonContent)
	if err != nil {
		// fmt.Errorf("ssb: combined idx failed to unmarshal json content of %s: %w", abstractMsg.Key().Ref(), err)
		// returning an error in this pipeline stops the processing,
		// i.e. broken messages stop all other indexing
		// not much to do here but continue with the next
		// these can be quite educational though (like root: bool)
		//
		// TODO: make a "forgiving" content type
		// which silently ignores invalid root: or tangle fields.
		// right now these don't end up in byType or tangles
		return nil
	}

	typeStr := jsonContent.Type
	if typeStr == "" {
		return fmt.Errorf("ssb: untyped message")
	}

	typedLog, err := slog.byType.Get(librarian.Addr("string:" + typeStr))
	if err != nil {
		return errors.Wrap(err, "error opening sublog")
	}

	_, err = typedLog.Append(seq)
	if err != nil {
		return errors.Wrap(err, "error updating byType sublog")
	}

	// tangles v1 and v2
	if jsonContent.Root != nil {
		addr := librarian.Addr(append([]byte("v1:"), jsonContent.Root.Hash...))
		tangleLog, err := slog.tangles.Get(addr)
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}
		_, err = tangleLog.Append(seq)
		if err != nil {
			return errors.Wrap(err, "error updating v1 tangle sublog")
		}
	}

	for tname, tip := range jsonContent.Tangles {
		addr := librarian.Addr(append([]byte("v2:"+tname+":"), tip.Root.Hash...))
		tangleLog, err := slog.tangles.Get(addr)
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}
		_, err = tangleLog.Append(seq)
		if err != nil {
			return errors.Wrap(err, "error updating v2 tangle sublog")
		}
	}

	return nil
}

// Close does nothing.
func (slog *combinedIndex) Close() error {
	return nil
}

// QuerySpec returns the query spec that queries the next needed messages from the log
func (slog *combinedIndex) QuerySpec() margaret.QuerySpec {
	slog.l.Lock()
	defer slog.l.Unlock()

	var seq margaret.BaseSeq

	if err := persist.Load(slog.file, &seq); err != nil {
		if errors.Cause(err) != io.EOF {
			return margaret.ErrorQuerySpec(err)
		}

		seq = margaret.SeqEmpty
	}

	if resN := slog.seqresolver.Seq() - 1; resN != seq.Seq() {
		err := fmt.Errorf("combined idx (has:%d, will: %d)", resN, seq.Seq())
		return margaret.ErrorQuerySpec(err)
	}

	return margaret.MergeQuerySpec(
		margaret.Gt(seq),
		margaret.SeqWrap(true),
	)
}

func (slog *combinedIndex) tryDecrypt(msg refs.Message, rxSeq margaret.Seq) ([]byte, error) {
	box1, box2, err := getBoxedContent(msg)
	if err != nil {
		// not super sure what the idea with the different skip errors was
		// these are _broken_ content-wise both kinds _should be ignored
		if err == errSkipBox1 || err == errSkipBox2 {
			return nil, errSkip
		}

		return nil, err
	}

	var (
		cleartext []byte
		idxAddr   librarian.Addr
	)

	/* as a help for re-indexing, keep track of all box1 and box2 messages.

	later we can get the set of messages we might need to re-index by
	1) taking type:special:box2
	2) ANDing it with the one of the author
	3) subtracting all the messages we _can_ read (private:box2:$ourFeed)
	*/
	if box1 != nil {
		idxAddr = librarian.Addr("special:box1")
	} else {
		idxAddr = librarian.Addr("special:box2")
	}

	boxTyped, err := slog.byType.Get(idxAddr)
	if err != nil {
		return nil, err
	}
	if _, err := boxTyped.Append(rxSeq.Seq()); err != nil {
		return nil, errors.Wrapf(err, "private: error marking type:box")
	}

	// try decrypt and pass on the clear text
	if box1 != nil {
		content, err := slog.boxer.DecryptBox1(box1)
		if err != nil {
			return nil, errSkip
		}

		idxAddr = librarian.Addr("box1:") + storedrefs.Feed(slog.self)
		cleartext = content
	} else if box2 != nil {
		content, err := slog.boxer.DecryptBox2(box2, msg.Author(), msg.Previous())
		if err != nil {
			return nil, errSkip
		}

		// instead by group root? could be PM... hmm
		// would be nice to keep multi-keypair support here
		// but might need to rethink the group manager
		idxAddr = librarian.Addr("box2:") + storedrefs.Feed(slog.self)
		cleartext = content
	} else {
		return nil, fmt.Errorf("tryDecrypt: not skipped but also not valid content")
	}

	userPrivs, err := slog.private.Get(idxAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "private/readidx: error opening priv sublog for")
	}
	if _, err := userPrivs.Append(rxSeq.Seq()); err != nil {
		return nil, errors.Wrapf(err, "private/readidx: error appending PM")
	}

	return cleartext, nil
}

var (
	errSkip     = fmt.Errorf("ssb: skip - not for us")
	errSkipBox1 = fmt.Errorf("ssb: skip box1 message")
	errSkipBox2 = fmt.Errorf("ssb: skip box2 message")
)

// returns either box1, box2 content or an error
// if err == errSkip, this message couldn't be decrypted
// everything else is a broken message (TODO: and should be... ignored?)
func getBoxedContent(msg refs.Message) ([]byte, []byte, error) {
	switch msg.Author().Algo {

	// on the _crappy_ format, we need to base64 decode the data
	case refs.RefAlgoFeedSSB1:
		input := msg.ContentBytes()
		if !(input[0] == '"' && input[len(input)-1] == '"') {
			return nil, nil, errSkipBox1 // not a json string
		}

		if bytes.HasSuffix(input[1:], []byte(".box\"")) {
			b64data := bytes.TrimSuffix(input[1:], []byte(".box\""))
			boxedData := make([]byte, base64.StdEncoding.DecodedLen(len(input)-6))
			n, err := base64.StdEncoding.Decode(boxedData, b64data)
			if err != nil {
				//err = errors.Wrap(err, "private/readidx: invalid b64 encoding")
				//level.Debug(pr.logger).Log("msg", "unboxLog b64 decode failed", "err", err)
				return nil, nil, errSkipBox1
			}
			return boxedData[:n], nil, nil
		} else if bytes.HasSuffix(input[1:], []byte(".box2\"")) {
			b64data := bytes.TrimSuffix(input[1:], []byte(".box2\""))
			boxedData := make([]byte, base64.StdEncoding.DecodedLen(len(input)-7))
			n, err := base64.StdEncoding.Decode(boxedData, b64data)
			if err != nil {
				err = errors.Wrap(err, "private/readidx: invalid b64 encoding")
				//level.Debug(pr.logger).Log("msg", "unboxLog b64 decode failed", "err", err)
				return nil, nil, errSkipBox1
			}
			return nil, boxedData[:n], nil
		} else {
			return nil, nil, errors.Errorf("private/ssb1: unknown content type: %q", input[len(input)-5:])
		}

		// gg supports pure binary data
	case refs.RefAlgoFeedGabby:
		mm, ok := msg.(multimsg.MultiMessage)
		if !ok {
			mmPtr, ok := msg.(*multimsg.MultiMessage)
			if !ok {
				err := errors.Errorf("private/readidx: error casting message. got type %T", msg)
				return nil, nil, err
			}
			mm = *mmPtr
		}
		tr, ok := mm.AsGabby()
		if !ok {
			return nil, nil, errors.Errorf("private/readidx: error getting gabby msg")
		}

		evt, err := tr.UnmarshaledEvent()
		if err != nil {
			return nil, nil, errors.Wrap(err, "private/readidx: error unpacking event from stored message")
		}
		if evt.Content.Type != gabbygrove.ContentTypeArbitrary {
			return nil, nil, errSkipBox2
		}

		var (
			prefixBox1 = []byte("box1:")
			prefixBox2 = []byte("box2:")
		)
		switch {
		case bytes.HasPrefix(tr.Content, prefixBox1):
			return tr.Content[5:], nil, nil
		case bytes.HasPrefix(tr.Content, prefixBox2):
			return nil, tr.Content[5:], nil
		default:
			return nil, nil, errors.Errorf("private/ssb1: unknown content type: %s", msg.Key().ShortRef())
		}

	default:
		err := errors.Errorf("private/readidx: unknown feed type: %s", msg.Author().Algo)
		return nil, nil, err
	}

}
