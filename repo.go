package sbot

import (
	"io"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
)

type Repo interface {
	io.Closer
	KeyPair() KeyPair
	Plugins() []Plugin
	BlobStore() BlobStore
	RootLog() margaret.Log        // the main log which contains all the feeds of individual users
	UserFeeds() multilog.MultiLog // use .Get(feedRef) to get a sublog just for that user
}
