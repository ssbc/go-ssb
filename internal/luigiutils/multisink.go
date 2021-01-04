// SPDX-License-Identifier: MIT

package luigiutils

import (
	"context"
	"errors"
	"fmt"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb/internal/neterr"
)

// MultiSink takes each message poured into it, and passes it on to all
// registered sinks.
//
// MultiSink is like luigi.Broadcaster but with context support.
// TODO(cryptix): cool utility! might want to move it to internal until we find  better place
type MultiSink struct {
	seq   int64
	sinks []*muxrpc.ByteSink
	ctxs  map[*muxrpc.ByteSink]context.Context
	until map[*muxrpc.ByteSink]int64

	isClosed bool
}

// var _ luigi.Sink = (*MultiSink)(nil)
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
) error {
	f.sinks = append(f.sinks, sink)
	f.ctxs[sink] = ctx
	f.until[sink] = until
	return nil
}

func (f *MultiSink) Unregister(
	sink *muxrpc.ByteSink,
) error {
	for i, s := range f.sinks {
		if sink != s {
			continue
		}
		f.sinks = append(f.sinks[:i], f.sinks[(i+1):]...)
		delete(f.ctxs, sink)
		delete(f.until, sink)
		return nil
	}
	return nil
}

// Count returns the number of registerd sinks
func (f *MultiSink) Count() uint {
	return uint(len(f.sinks))
}

func (f *MultiSink) Close() error {
	f.isClosed = true
	return nil
}

func (f *MultiSink) Send(_ context.Context, msg []byte) error {
	if f.isClosed {
		return luigi.EOS{}
	}
	f.seq++

	var deadFeeds []*muxrpc.ByteSink

	for i, s := range f.sinks {
		_, err := s.Write(msg)
		if err != nil {
			if muxrpc.IsSinkClosed(err) || errors.Is(err, context.Canceled) || neterr.IsConnBrokenErr(err) {
				deadFeeds = append(deadFeeds, s)
				continue
			}
			return fmt.Errorf("MultiSink: failed to pour into sink #%d: %w", i, err)
		}
		if f.until[s] <= f.seq {
			deadFeeds = append(deadFeeds, s)
		}
	}

	for _, feed := range deadFeeds {
		f.Unregister(feed)
	}

	return nil
}
