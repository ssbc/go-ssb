package multilogs

import (
	"encoding/json"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

// IndexToplevel is the name of the key-value index.
// It indexes all top-level string values so we can search for them.
const IndexToplevel = "toplevel"

func TopLevelExtract(value interface{}) (map[string]string, error) {
	msg, ok := value.(message.StoredMessage)
	if !ok {
		return nil, errors.Errorf("error casting message. got type %T, expected %T", value, msg)
	}

	// string content means encrypted message; skip
	if msg.Raw[0] == '"' {
		return nil, nil
	}

	// now we should only see object messages and everything else is an error

	var contentMsg struct {
		Content map[string]interface{} `json:"content"`
	}

	err := json.Unmarshal(msg.Raw, &contentMsg)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding message")
	}

	content := make(map[string]string)
	for k, v := range contentMsg.Content {
		str, ok := v.(string)
		if !ok {
			continue
		}

		content[k] = str
	}

	return content, nil
}

// OpenToplevel returns a multilog that has as index the top level key value pairs of messages. It only indexes pairs where the value is string and not longer than maxLength.
func OpenToplevel(r repo.Interface, maxLength int) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return openGeneric(r, IndexToplevel, NewStoredMessageRawExtractor(
		NewJSONDecodeToMapExtractor(
			NewTraverseExtractor([]string{"content"},
				genericExtractor(StringsExtractor)))), maxLength)
}
