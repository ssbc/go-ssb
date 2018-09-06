package repo

import (
	"io"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/graph"
)

type Interface interface {
	io.Closer
	GetPath(...string) string
	KeyPair() sbot.KeyPair
	Plugins() []sbot.Plugin
	BlobStore() sbot.BlobStore
	RootLog() margaret.Log // the main log which contains all the feeds of individual users
	Builder() graph.Builder
}
