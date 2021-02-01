// SPDX-License-Identifier: MIT

package luigiutils

import (
	"context"
	"sync"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
)

// MultiSink takes each message poured into it, and passes it on to all
// registered sinks.
//
// MultiSink is like luigi.Broadcaster but with context support.
type MultiSink struct {
	seq int64

	mu    sync.Mutex
	sinks []*muxrpc.ByteSink
	ctxs  map[*muxrpc.ByteSink]context.Context
	until map[*muxrpc.ByteSink]int64

	isClosed bool
}

var _ margaret.Seq = (*MultiSink)(nil)

func NewMultiSink(seq int64) *MultiSink {
	return &MultiSink{
		seq:   seq,
		ctxs:  make(map[*muxrpc.ByteSink]context.Context),
		until: make(map[*muxrpc.ByteSink]int64),
	}
}

func (f *MultiSink) Seq() int64 {
	return f.seq
}

// Register adds a sink to propagate messages to upto the 'until'th sequence.
func (f *MultiSink) Register(
	ctx context.Context,
	sink *muxrpc.ByteSink,
	until int64,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sinks = append(f.sinks, sink)
	f.ctxs[sink] = ctx
	f.until[sink] = until
}

func (f *MultiSink) Unregister(
	sink *muxrpc.ByteSink,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unregister(sink)
}

func (f *MultiSink) unregister(
	sink *muxrpc.ByteSink,
) {
	for i, s := range f.sinks {
		if sink != s {
			continue
		}
		f.sinks = append(f.sinks[:i], f.sinks[(i+1):]...)
		delete(f.ctxs, sink)
		delete(f.until, sink)
	}
}

// Count returns the number of registerd sinks
func (f *MultiSink) Count() uint {
	f.mu.Lock()
	defer f.mu.Unlock()
	return uint(len(f.sinks))
}

func (f *MultiSink) Close() error {
	f.isClosed = true
	return nil
}

func (f *MultiSink) Send(msg []byte) {
	if f.isClosed {
		return
	}
	f.seq++

	f.mu.Lock()
	defer f.mu.Unlock()

	var deadFeeds []*muxrpc.ByteSink

	for _, s := range f.sinks {
		_, err := s.Write(msg)
		if err != nil {
			deadFeeds = append(deadFeeds, s)
		}
		if f.until[s] <= f.seq {
			deadFeeds = append(deadFeeds, s)
		}
	}

	for _, feed := range deadFeeds {
		f.unregister(feed)
	}
}
