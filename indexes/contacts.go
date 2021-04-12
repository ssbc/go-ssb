// SPDX-License-Identifier: MIT

package indexes

import (
	"fmt"

	"github.com/dgraph-io/badger"
	"go.cryptoscope.co/librarian"
	kitlog "go.mindeco.de/log"

	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/repo"
)

const FolderNameContacts = "contacts"

func OpenContacts(log kitlog.Logger, r repo.Interface) (graph.Builder, librarian.SeqSetterIndex, librarian.SinkIndex, error) {
	var builder graph.IndexingBuilder
	f := func(db *badger.DB) (librarian.SeqSetterIndex, librarian.SinkIndex) {
		builder = graph.NewBuilder(kitlog.With(log, "module", "graph"), db)
		return builder.OpenIndex()
	}

	_, idx, updateSink, err := repo.OpenBadgerIndex(r, FolderNameContacts, f)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error getting contacts index: %w", err)
	}

	return builder, idx, updateSink, nil
}
