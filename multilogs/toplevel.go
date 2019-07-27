package multilogs

import (
	"github.com/dgraph-io/badger"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/multilogs/mlutil"
	"go.cryptoscope.co/ssb/repo"
)

// IndexToplevel is the name of the key-value index.
// It indexes all top-level string values so we can search for them.
const IndexToplevel = "toplevel"

// OpenToplevel returns a multilog that has as index the top level key value pairs of messages. It only indexes pairs where the value is string and not longer than maxLength.
func OpenToplevel(r repo.Interface, maxLength int) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return mlutil.OpenGeneric(r, IndexToplevel, mlutil.NewStoredMessageRawExtractor(
		mlutil.NewJSONDecodeToMapExtractor(
			mlutil.NewTraverseExtractor([]string{"content"},
				mlutil.GenericExtractor(mlutil.StringsExtractor)))), maxLength)
}
