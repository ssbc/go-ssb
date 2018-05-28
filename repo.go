package sbot

import (
	"cryptoscope.co/go/secretstream/secrethandshake"
	"cryptoscope.co/go/margaret"
)

type Repo interface {
	KeyPair() secrethandshake.EdKeyPair
	Plugins() []Plugin
	BlobStore() BlobStore
	Log() margaret.Log
}
