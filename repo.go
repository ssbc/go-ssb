package sbot

import (
	"io"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
)

type Repo interface {
	io.Closer
	KeyPair() KeyPair
	Plugins() []Plugin
	BlobStore() BlobStore
	Log() margaret.Log
	GossipIndex() librarian.SeqSetterIndex
	KnownFeeds() (map[string]margaret.BaseSeq, error) // cant use FeedRef as key..
	FeedSeqs(FeedRef) ([]margaret.BaseSeq, error)
}
