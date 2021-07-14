// SPDX-License-Identifier: MIT

package indexes

import (
	"github.com/dgraph-io/badger/v3"
	librarian "go.cryptoscope.co/margaret/indexes"
	kitlog "go.mindeco.de/log"

	"go.cryptoscope.co/ssb/graph"
)

func OpenContacts(log kitlog.Logger, db *badger.DB) (graph.Builder, librarian.SeqSetterIndex, librarian.SinkIndex) {
	builder := graph.NewBuilder(kitlog.With(log, "module", "graph"), db)
	seqSetter, updateSink := builder.OpenIndex()

	return builder, seqSetter, updateSink
}
