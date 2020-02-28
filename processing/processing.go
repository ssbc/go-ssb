package processing

import (
	"context"
	"encoding/json"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
)

type MessageProcessor interface {
	ProcessMessage(ctx context.Context, msg ssb.Message, seq margaret.Seq) error
	Close(ctx context.Context) error
}

type ContentProcessorFunc func(content map[string]interface{}) ([]string, error)

type ContentProcessor struct {
	f    ContentProcessorFunc
	mlog multilog.MultiLog
}

func (cp ContentProcessor) ProcessMessage(ctx context.Context, msg ssb.Message, seq margaret.Seq) error {
	contentBs := msg.ContentBytes()
	if len(contentBs) == 0 || contentBs[0] != '{' {
		return nil
	}

	content := make(map[string]interface{})

	err := json.Unmarshal(contentBs, &content)
	if err != nil {
		return err
	}

	strings, err := cp.f(content)
	if err != nil {
		return err
	}

	for _, str := range strings {
		slog, err := cp.mlog.Get(librarian.Addr(str))
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
