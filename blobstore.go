package sbot

import "io"

type BlobStore interface {
	Get(ref *BlobRef) (io.Reader, error)
	Put(blob io.Reader) (*BlobRef, error)
}
