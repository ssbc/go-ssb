package repo

import (
	"io"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/sbot"
)

type Interface interface {
	io.Closer
	KeyPair() sbot.KeyPair
	Plugins() []sbot.Plugin
	BlobStore() sbot.BlobStore
	RootLog() margaret.Log        // the main log which contains all the feeds of individual users
	UserFeeds() multilog.MultiLog // use .Get(feedRef) to get a sublog just for that user
	IsFollowing(fr *sbot.FeedRef) ([]*sbot.FeedRef, error)
	Makegraph() (*Graph, error)
}
