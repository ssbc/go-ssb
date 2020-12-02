package luigiutils

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"
)

type sinkWrapper func(luigi.Sink) luigi.Sink

func MutliSinkWithWrappedSink(fn sinkWrapper) func(t *testing.T) {
	return func(t *testing.T) {
		r := require.New(t)
		ctx := context.TODO()

		mSink := NewMultiSink(0)

		// setup some things that will fail one after the other
		var fSinks []*failingSink
		for i := 10; i > 0; i-- {
			newSink := failingSink(i)
			fSinks = append(fSinks, &newSink)

			err := mSink.Register(ctx, fn(&newSink), 100)
			r.NoError(err, "failed to register sink %d", i)
		}
		r.EqualValues(10, mSink.Count(), "not all sinks registerd")

		for i := 10; i >= 0; i-- {
			err := mSink.Pour(ctx, fmt.Sprintf("hello, world! %d", i))
			r.NoError(err)
			t.Log(i, err, mSink.Count())
		}

		r.EqualValues(0, mSink.Count(), "still registerd sinks")
	}
}

func TestMultiLogSinkWrapped(t *testing.T) {

	noop := func(s luigi.Sink) luigi.Sink {
		return s
	}
	t.Run("no-op", MutliSinkWithWrappedSink(noop))

	luigiFuncSink := func(s luigi.Sink) luigi.Sink {
		fsnk := luigi.FuncSink(func(ctx context.Context, v interface{}, closeErr error) error {
			if closeErr != nil {
				return closeErr
			}
			return s.Pour(ctx, v)
		})
		// need to take the pointer otherwise it's an unhashable type!
		// TODO: could refactor multiSink not to use map[luigi.Sink] internally
		return &fsnk
	}
	t.Run("luigi FuncSink", MutliSinkWithWrappedSink(luigiFuncSink))

	mfrSinkMap := func(s luigi.Sink) luigi.Sink {
		return mfr.SinkMap(s, func(ctx context.Context, v interface{}) (interface{}, error) {
			return v, nil
		})
	}
	t.Run("map-filter-reduce SinkMap", MutliSinkWithWrappedSink(mfrSinkMap))

}

type failingSink int

func (f *failingSink) Close() error { return nil }

func (f *failingSink) Pour(_ context.Context, msg interface{}) error {
	if int(*f) <= 0 {
		return luigi.EOS{}
	}
	*f--
	return nil
}
