package sbot

import (
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
)

type Repo interface {
	KeyPair() KeyPair
	Plugins() []Plugin
	BlobStore() BlobStore
	Log() margaret.Log
	GossipIndex() librarian.SeqSetterIndex
	KnownFeeds() (map[string]margaret.Seq, error) // cant use FeedRef as key..
	FeedSeqs(FeedRef) ([]margaret.Seq, error)
}
