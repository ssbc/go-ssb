// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"context"
	"fmt"
	"io"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/v2"

	refs "go.mindeco.de/ssb-refs"
)

const (
	// BlobStoreOpPut is used in put notifications
	BlobStoreOpPut BlobStoreOp = "put"

	// BlobStoreOpRm is used in remove notifications
	BlobStoreOpRm BlobStoreOp = "rm"
)

// BlobStore is the interface of our blob store
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o mock/blobstore.go . BlobStore
type BlobStore interface {
	// Get returns a reader of the blob with given ref.
	Get(ref refs.BlobRef) (io.ReadCloser, error)

	// Put stores the data in the reader in the blob store and returns the address.
	Put(blob io.Reader) (refs.BlobRef, error)

	// PutExpected makes sure the added blob really is the passedBlobref
	// helpful for want/get operations which don't want to waste resources
	// PutExpected(io.Reader, *refs.BlobRef) error

	// Delete deletes a blob from the blob store.
	Delete(ref refs.BlobRef) error

	// List returns a source of the refs of all stored blobs.
	List() luigi.Source

	// Size returns the size of the blob with given ref.
	Size(ref refs.BlobRef) (int64, error)

	// Register allows to get notified when the store changes
	BlobStoreBroadcaster
}

// BlobStoreEmitter emits events of the blobstore
type BlobStoreEmitter interface {
	EmitBlob(BlobStoreNotification) error
	io.Closer
}

// BlobStoreBroadcaster allows for registration to changes
type BlobStoreBroadcaster interface {
	Register(sink BlobStoreEmitter) CancelFunc
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o mock/wantmanager.go . WantManager

// WantManager manages fetching blobs that are not stored yet
type WantManager interface {
	io.Closer

	BlobWantsBroadcaster

	Want(ref refs.BlobRef) error
	Wants(ref refs.BlobRef) bool
	WantWithDist(ref refs.BlobRef, dist int64) error
	//Unwant(ref refs.BlobRef) error
	CreateWants(context.Context, *muxrpc.ByteSink, muxrpc.Endpoint) luigi.Sink

	AllWants() []BlobWant
}

// CancelFunc is ised to unsubscribe from updates
type CancelFunc func()

// BlobWantsEmitter emits events of the want manager
type BlobWantsEmitter interface {
	EmitWant(BlobWant) error
	io.Closer
}

// BlobWantsBroadcaster allows for registration to changes from the want system
type BlobWantsBroadcaster interface {
	Register(sink BlobWantsEmitter) CancelFunc
}

// BlobWant represents a want for some blob, send by some peer.
// It can inform about avalibily (via a positive distance, which represents the size of the blob)
// or that a peer wants a blob (< then -1 for sympathy from other peers)
type BlobWant struct {
	Ref refs.BlobRef

	// if Dist is negative, it is the hop count to the original wanter.
	// if it is positive, it is the size of the blob.
	Dist int64
}

func (w BlobWant) String() string {
	return fmt.Sprintf("%s:%d", w.Ref.ShortSigil(), w.Dist)
}

// BlobStoreNotification contains info on a single change of the blob store.
// Op is either "rm" or "put".
type BlobStoreNotification struct {
	Op  BlobStoreOp
	Ref refs.BlobRef

	Size int64
}

func (bn BlobStoreNotification) String() string {
	s := bn.Op.String() + ": " + bn.Ref.Sigil()
	if bn.Size > 0 {
		s += fmt.Sprintf(" (size: %d)", bn.Size)
	}
	return s
}

// BlobStoreOp specifies the operation in a blob store notification.
type BlobStoreOp string

// String returns the string representation of the operation.
func (op BlobStoreOp) String() string {
	return string(op)
}
