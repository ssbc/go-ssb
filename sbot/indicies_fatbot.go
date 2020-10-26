package sbot

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

	"github.com/keks/persist"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/repo"
	gabbygrove "go.mindeco.de/ssb-gabbygrove"
	refs "go.mindeco.de/ssb-refs"
)

func (bot *Sbot) newApplicationIndex() error {

	statePath := repo.New(bot.repoPath).GetPath(repo.PrefixMultiLog, "combined-state.json")
	mode := os.O_RDWR | os.O_EXCL
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		mode |= os.O_CREATE
	}
	os.MkdirAll(filepath.Dir(statePath), 0700)
	idxStateFile, err := os.OpenFile(statePath, mode, 0700)
	if err != nil {
		return errors.Wrap(err, "error opening state file")
	}

	idx := &applicationIdx{
		bot:  bot,
		file: idxStateFile,
		l:    &sync.Mutex{},
	}
	bot.serveIndex("combined", idx)
	return nil
}

type applicationIdx struct {
	bot  *Sbot
	file *os.File
	l    *sync.Mutex
}

// Pour calls the processing function to add a value to a sublog.
func (slog *applicationIdx) Pour(ctx context.Context, v interface{}) error {
	slog.l.Lock()
	defer slog.l.Unlock()

	seq := v.(margaret.SeqWrapper)

	// todo: defer state save!?
	err := persist.Save(slog.file, seq.Seq())
	if err != nil {
		return errors.Wrap(err, "error saving current sequence number")
	}

	if isNulled, ok := v.(error); ok {
		if margaret.IsErrNulled(isNulled) {
			return nil
		}
		return isNulled
	}

	abstractMsg, ok := v.(refs.Message)
	if !ok {
		return errors.Errorf("error casting message. got type %T", v)
	}

	author := abstractMsg.Author()

	authorLog, err := slog.bot.Users.Get(author.StoredAddr())
	if err != nil {
		return errors.Wrap(err, "error opening sublog")
	}
	_, err = authorLog.Append(seq)
	if err != nil {
		return errors.Wrap(err, "error updating author sublog")
	}

	// decrypt box 1 & 2
	content := abstractMsg.ContentBytes()
	if content[0] != '{' { // assuming all other content is json objects
		cleartext, err := slog.tryDecrypt(abstractMsg, seq.Seq())
		if err != nil {
			if err == errSkip {
				return nil
			}
			return err
		}
		content = cleartext
	}

	// by type:
	var jsonContent struct {
		Type string

		Root *refs.MessageRef

		Tangles refs.Tangles
	}

	err = json.Unmarshal(content, &jsonContent)
	typeStr := jsonContent.Type
	if err != nil {
		return err
	}

	if typeStr == "" {
		return fmt.Errorf("ssb: untyped message")
	}

	typedLog, err := slog.bot.ByType.Get(librarian.Addr(typeStr))
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
		tangleLog, err := slog.bot.Tangles.Get(addr)
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}
		_, err = tangleLog.Append(seq)
		if err != nil {
			return errors.Wrap(err, "error updating v1 tangle sublog")
		}
	}

	for tname, tip := range jsonContent.Tangles {
		addr := librarian.Addr(append([]byte("v2:"+tname), tip.Root.Hash...))
		tangleLog, err := slog.bot.Tangles.Get(addr)
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
func (slog *applicationIdx) Close() error { return nil }

// QuerySpec returns the query spec that queries the next needed messages from the log
func (slog *applicationIdx) QuerySpec() margaret.QuerySpec {
	slog.l.Lock()
	defer slog.l.Unlock()

	var seq margaret.BaseSeq

	if err := persist.Load(slog.file, &seq); err != nil {
		if errors.Cause(err) != io.EOF {
			return margaret.ErrorQuerySpec(err)
		}

		seq = margaret.SeqEmpty
	}

	return margaret.MergeQuerySpec(
		margaret.Gt(seq),
		margaret.SeqWrap(true),
	)
}

func (slog *applicationIdx) tryDecrypt(msg refs.Message, rxSeq margaret.Seq) ([]byte, error) {
	box1, box2, err := getBoxedContent(msg)
	if err != nil {
		// not super sure what the idea with the different skip errors was
		// these are _broken_ content-wise both kinds _should be ignored
		if err == errSkipBox1 {
			//slog.bot.ByTypes.Get(librarian.Addr("special:boxed1"))
			return nil, errSkip
		}

		if err == errSkipBox2 {
			//slog.bot.ByTypes.Get(librarian.Addr("special:boxed2"))
			return nil, errSkip
		}

		return nil, err
	}

	var (
		cleartext []byte
		idxAddr   librarian.Addr
		retErr    error
	)
	if box1 != nil {
		content, err := slog.bot.Groups.DecryptBox1(box1)
		if err != nil {
			idxAddr = librarian.Addr("notForUs:box1")
			retErr = errSkip
		} else {
			idxAddr = librarian.Addr("box1:TODO:ME") // + kp.Id.StoredAddr()
			cleartext = content
		}
	} else if box2 != nil {
		content, err := slog.bot.Groups.DecryptBox2(box2, msg.Author(), msg.Previous())
		if err != nil {
			idxAddr = librarian.Addr("notForUs:box2")
			retErr = errSkip
		} else {
			// instead by group root? could be PM... hmm
			// would be nice to keep multi-keypair support here but might need rething of the gorups manager
			idxAddr = librarian.Addr("box2:TODO:ME") // + kp.Id.StoredAddr()
			cleartext = content
		}
	} else {
		return nil, fmt.Errorf("tryDecrypt: not skipped but also not valid content")
	}

	userPrivs, err := slog.bot.Private.Get(idxAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "private/readidx: error opening priv sublog for")
	}
	if _, err := userPrivs.Append(rxSeq.Seq()); err != nil {
		return nil, errors.Wrapf(err, "private/readidx: error appending PM")
	}

	return cleartext, retErr
}

var (
	errSkip     = fmt.Errorf("ssb: skip  - not for us")
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
		if bytes.HasPrefix(tr.Content, prefixBox1) {
			return tr.Content[5:], nil, nil
		} else if bytes.HasPrefix(tr.Content, prefixBox2) {
			return nil, tr.Content[5:], nil
		} else {
			return nil, nil, errors.Errorf("private/ssb1: unknown content type: %s", msg.Key().ShortRef())
		}

	default:
		err := errors.Errorf("private/readidx: unknown feed type: %s", msg.Author().Algo)
		//level.Warn(pr.logger).Log("msg", "unahndled type", "err", err)
		return nil, nil, err
	}

}
