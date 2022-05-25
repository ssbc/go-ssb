// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package broadcasts

import (
	"sync"

	"github.com/ssbc/go-ssb"
	"github.com/ssbc/go-ssb/internal/multierror"
)

type BlobStoreBroadcast struct {
	mu    *sync.Mutex
	sinks map[*ssb.BlobStoreEmitter]struct{}
}

func NewBlobStoreBroadcast() *BlobStoreBroadcast {
	return &BlobStoreBroadcast{
		mu:    &sync.Mutex{},
		sinks: make(map[*ssb.BlobStoreEmitter]struct{}),
	}
}

func (bcst *BlobStoreBroadcast) Register(sink ssb.BlobStoreEmitter) ssb.CancelFunc {
	bcst.mu.Lock()
	defer bcst.mu.Unlock()
	bcst.sinks[&sink] = struct{}{}

	return func() {
		bcst.mu.Lock()
		defer bcst.mu.Unlock()
		delete(bcst.sinks, &sink)
		sink.Close()
	}
}

func (bcst *BlobStoreBroadcast) EmitBlob(nf ssb.BlobStoreNotification) error {
	bcst.mu.Lock()
	for s := range bcst.sinks {
		err := (*s).EmitBlob(nf)
		if err != nil {
			delete(bcst.sinks, s)
		}
	}
	bcst.mu.Unlock()

	return nil
}

func (bcst *BlobStoreBroadcast) Close() error {
	var sinks []ssb.BlobStoreEmitter

	bcst.mu.Lock()
	defer bcst.mu.Unlock()

	sinks = make([]ssb.BlobStoreEmitter, 0, len(bcst.sinks))

	for sink := range bcst.sinks {
		sinks = append(sinks, *sink)
	}

	var (
		wg sync.WaitGroup
		me multierror.List
	)

	// might be fine without the waitgroup and concurrency

	wg.Add(len(sinks))
	for _, sink_ := range sinks {
		go func(sink ssb.BlobStoreEmitter) {
			defer wg.Done()

			err := sink.Close()
			if err != nil {
				me.Errs = append(me.Errs, err)
				return
			}
		}(sink_)
	}
	wg.Wait()

	if len(me.Errs) == 0 {
		return nil
	}

	return me
}

type BlobStoreFuncEmitter func(not ssb.BlobStoreNotification) error

func (e BlobStoreFuncEmitter) EmitBlob(not ssb.BlobStoreNotification) error {
	return e(not)
}

func (e BlobStoreFuncEmitter) Close() error {
	return nil
}
