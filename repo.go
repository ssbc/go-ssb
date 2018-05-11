package sbot

import "cryptoscope.co/go/secretstream/secrethandshake"

type Repo interface {
	KeyPair() secrethandshake.EdKeyPair
	Plugins() []Plugin
	BlobStore() BlobStore
}
