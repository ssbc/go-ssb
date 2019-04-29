// Package numberedfeeds is a utitily wrapper for go-ssb types around
// go.mindeco.de/rumorlog
// giving each TFK encoded feed a unique sequence number
package numberedfeeds

import (
	"go.mindeco.de/rumorlog/trie"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

type Index struct {
	trie trie.TrieIndex
}

func New(path string) (*Index, error) {
	var (
		idx Index
		err error
	)

	idx.trie, err = trie.New(path)
	if err != nil {
		return nil, err
	}

	return &idx, nil
}

func (idx Index) NumberFor(r *refs.FeedRef) (uint64, error) {
	enc, err := tfk.Encode(r)
	if err != nil {
		return 0, err
	}
	return idx.trie.GetSequence(enc, true)
}

func (idx Index) FeedFor(n uint64) (*refs.FeedRef, error) {
	b, err := idx.trie.GetKey(n)
	if err != nil {
		return nil, err
	}

	var f tfk.Feed
	err = f.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	return f.Feed(), nil
}

func (idx Index) Close() error {
	return idx.trie.Close()
}
