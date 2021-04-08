package broadcasts

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
)

type BlobsInterface interface {
	Register(dst *WantUpdater) CancelFunc

	WantUpdater
}

type WantUpdater interface {
	Update(context.Context, ssb.BlobWant) error
	io.Closer
}

// muxrpc shim
type MuxrpcBlobUpdater struct {
	sink *muxrpc.ByteSink
	enc  *json.Encoder
}

func NewUpdaterFromMUXRPCSink(sink *muxrpc.ByteSink) *MuxrpcBlobUpdater {
	sink.SetEncoding(muxrpc.TypeJSON)
	return &MuxrpcBlobUpdater{
		sink: sink,
		enc:  json.NewEncoder(sink),
	}
}

func (u MuxrpcBlobUpdater) Update(_ context.Context, want ssb.BlobWant) error {
	return u.enc.Encode(want)
}

func (u MuxrpcBlobUpdater) Close() error {
	return u.sink.CloseWithError(nil)
}

// blob internals shim

type BlobUpdateFilterFunc func(ssb.BlobWant) bool

// NewFilteredUpdater returns an updater that calls u if fn returns true for that update
func NewFilteredUpdater(u WantUpdater, fn BlobUpdateFilterFunc) WantUpdater {
	return filteredUpdater{
		updater:  u,
		filterFn: fn,
	}
}

type filteredUpdater struct {
	updater  WantUpdater
	filterFn BlobUpdateFilterFunc
}

func (u filteredUpdater) Update(ctx context.Context, want ssb.BlobWant) error {
	if !u.filterFn(want) {
		return nil
	}
	return u.Update(ctx, want)
}

func (u filteredUpdater) Close() error { return u.updater.Close() }

// interface assertions
var (
	_ BlobsInterface = (*BlobUpdates)(nil)
	_ WantUpdater    = (*MuxrpcBlobUpdater)(nil)
)

// NewBlobUpdater
func NewBlobUpdater() *BlobUpdates {
	return &BlobUpdates{
		sinks: make(mapType),
	}
}

type BlobUpdates struct {
	mu    sync.Mutex
	sinks mapType
}

type mapType map[*WantUpdater]struct{}

// CancelFunc can be used to stop the subscription anc close the stream
type CancelFunc func()

// Register implements the Broadcast interface.
func (bcst *BlobUpdates) Register(sink *WantUpdater) CancelFunc {
	bcst.mu.Lock()
	defer bcst.mu.Unlock()

	bcst.sinks[sink] = struct{}{}

	return func() {
		bcst.mu.Lock()
		defer bcst.mu.Unlock()
		delete(bcst.sinks, sink)
		(*sink).Close()
	}
}

// Pour implements the Sink interface.
func (bcst *BlobUpdates) Update(ctx context.Context, want ssb.BlobWant) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bcst.mu.Lock()
	defer bcst.mu.Unlock()

	for sink := range bcst.sinks {
		err := (*sink).Update(ctx, want)
		if err != nil {
			delete(bcst.sinks, sink)
		}
	}

	return nil
}

// Close implements the Sink interface.
func (bcst *BlobUpdates) Close() error {
	bcst.mu.Lock()
	defer bcst.mu.Unlock()

	for sink := range bcst.sinks {
		(*sink).Close()
	}

	// clear the map
	bcst.sinks = make(mapType, 0)

	return nil
}
