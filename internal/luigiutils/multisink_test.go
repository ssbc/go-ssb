// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

//go:build ignore
// +build ignore

// TODO: refactor to refs.Message

package luigiutils

import (
	"context"
	"fmt"
	"io"
	"testing"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/v2"

	"github.com/stretchr/testify/require"
)

type sinkWrapper func(io.Writer) luigi.Sink

func MutliSinkWithWrappedSink(fn sinkWrapper) func(t *testing.T) {
	return func(t *testing.T) {
		r := require.New(t)
		ctx := context.TODO()

		mSink := NewMultiSink(0)

		// setup some things that will fail one after the other
		var fSinks []*failingWriter
		for i := 10; i > 0; i-- {
			newSink := failingWriter(i)
			fSinks = append(fSinks, &newSink)

			mSink.Register(ctx, fn(&newSink), 100)
		}
		r.EqualValues(10, mSink.Count(), "not all sinks registerd")

		for i := 10; i >= 0; i-- {
			body := fmt.Sprintf("hello, world! %d", i)

			mSink.Send([]byte(body))

			t.Log(i, mSink.Count())
		}

		r.EqualValues(0, mSink.Count(), "still registerd sinks")
	}
}

func TestMultiLogSinkWrapped(t *testing.T) {

	noop := func(w io.Writer) *muxrpc.ByteSink {
		return muxrpc.NewTestSink(w)
	}
	t.Run("no-op", MutliSinkWithWrappedSink(noop))

	// luigiFuncSink := func(w io.Writer) *muxrpc.ByteSink {
	// 	fsnk := luigi.FuncSink(func(ctx context.Context, v interface{}, closeErr error) error {
	// 		if closeErr != nil {
	// 			return closeErr
	// 		}
	// 		return s.Pour(ctx, v)
	// 	})
	// 	// need to take the pointer otherwise it's an unhashable type!
	// 	// TODO: could refactor multiSink not to use map[luigi.Sink] internally
	// 	return &fsnk
	// }
	// t.Run("luigi FuncSink", MutliSinkWithWrappedSink(luigiFuncSink))

	// mfrSinkMap := func(w io.Writer) *muxrpc.ByteSink {
	// 	return mfr.SinkMap(s, func(ctx context.Context, v interface{}) (interface{}, error) {
	// 		return v, nil
	// 	})
	// }
	// t.Run("map-filter-reduce SinkMap", MutliSinkWithWrappedSink(mfrSinkMap))

}

type failingWriter int

func (f *failingWriter) Close() error { return nil }

func (f *failingWriter) Write(b []byte) (int, error) {
	if int(*f) <= 0 {
		return -1, io.ErrUnexpectedEOF
	}
	*f--
	return len(b), nil
}
