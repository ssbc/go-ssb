package repo

import (
	"io"

	"go.cryptoscope.co/sbot"
)

type Interface interface {
	io.Closer
	GetPath(...string) string
	KeyPair() sbot.KeyPair
	BlobStore() sbot.BlobStore
}
