package processing

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/keks/persist"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
)

// MessageProcessor is the central abstraction for processing messages.
// Any specific index can implement this and be hooked into an Index.
// It will be called with each message and corresponding sequence number.
type MessageProcessor interface {
	ProcessMessage(ctx context.Context, msg ssb.Message, seq margaret.Seq) error
	CurrentSeq() (margaret.Seq, error)
	Close(ctx context.Context) error
}

// ContentProcessorFunc makes it easy to write indexers that extract subsets of messages.
// Each subset is described by a string, and each message can be part of multiple subsets.
// A content processor function extracts the list of subsets the message belongs to.
type ContentProcessorFunc func(content map[string]interface{}) ([]string, error)

// ContentProcessor maintains the subsets for a particular ContentProcessorFunc Func.
// Each subset corresponds to a sublog in MLog.
// For each message the ContentProcessorFunc Func is called and the Multilog MLog is
// updated to mark that the message with the respective sequence number is in
// the subset.
//
// Examples for subsets are "all messages with type post" or "all messages with
// gatherings tangle root %abc.sha256".
type ContentProcessor struct {
	Func      ContentProcessorFunc
	MLog      multilog.MultiLog
	StateFile *os.File

	l sync.Mutex
}

// ProcessMessage indexes a message.
func (cp *ContentProcessor) ProcessMessage(ctx context.Context, msg ssb.Message, seq margaret.Seq) (err error) {
	cp.l.Lock()
	defer cp.l.Unlock()
	defer func() {
		if cp.StateFile != nil {
			saveErr := persist.Save(cp.StateFile, seq.Seq())
			if err == nil && saveErr != nil {
				err = saveErr
			}
		}
	}()

	contentBs := msg.ContentBytes()
	if len(contentBs) == 0 || contentBs[0] != '{' {
		return nil
	}

	content := make(map[string]interface{})
	err = json.Unmarshal(contentBs, &content)
	if err != nil {
		return err
	}

	strings, err := cp.Func(content)
	if err != nil {
		return err
	}

	for _, str := range strings {
		slog, err := cp.MLog.Get(librarian.Addr(str))
		if err != nil {
			return err
		}

		_, err = slog.Append(seq)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cp *ContentProcessor) CurrentSeq() (margaret.Seq, error) {
	if cp.StateFile == nil {
		return margaret.BaseSeq(-1), nil
	}

	cp.l.Lock()
	defer cp.l.Unlock()

	var (
		seq margaret.BaseSeq
		err = persist.Load(cp.StateFile, &seq)
	)

	return seq, err
}
