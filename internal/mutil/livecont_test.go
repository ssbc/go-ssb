package mutil

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	marmock "go.cryptoscope.co/margaret/mock"
)

func TestLiveResumerSimple(t *testing.T) {
	f, err := ioutil.TempFile(".", "TestLiveResumer.simple")
	require.NoError(t, err, "create temp file")

	defer func() {
		if !t.Failed() {
			os.Remove(f.Name())
		}
	}()

	var (
		calledCount int
		testValue   interface{}      = "test data"
		testSeq     margaret.BaseSeq = 0
		sink        luigi.FuncSink   = func(ctx context.Context, v interface{}, err error) error {
			require.NoError(t, err, "passed into funcsink")

			require.Equal(t, calledCount, 0, "call count mismatch")
			calledCount++

			require.Equal(t, v, testValue, "test data mismatch")

			return nil
		}
	)

	lr, err := NewLiveResumer(f, sink)
	require.NoError(t, err, "new live resumer")

	err = lr.Pour(context.TODO(), margaret.WrapWithSeq(testValue, testSeq))
	require.NoError(t, err, "pour")

	require.Equal(t, lr.seq, testSeq.Seq())

	var (
		qs  = lr.QuerySpec()
		qry = &marmock.FakeQuery{}
	)

	err = qs(qry)
	require.NoError(t, err)

	require.Equal(t, qry.GtCallCount(), 1)
	require.Equal(t, qry.GtArgsForCall(0), margaret.BaseSeq(0))

	require.Equal(t, qry.GteCallCount(), 0)
	require.Equal(t, qry.LimitCallCount(), 0)
	require.Equal(t, qry.LiveCallCount(), 0)
	require.Equal(t, qry.LtCallCount(), 0)
	require.Equal(t, qry.LteCallCount(), 0)

	require.Equal(t, qry.SeqWrapCallCount(), 1)
	require.Equal(t, qry.SeqWrapArgsForCall(0), true)
}

func TestLiveResumerMultiple(t *testing.T) {
	f, err := ioutil.TempFile(".", "TestLiveResumer.simple")
	require.NoError(t, err, "create temp file")

	defer func() {
		if !t.Failed() {
			os.Remove(f.Name())
		}
	}()

	var (
		calledCount int
		testData    = []interface{}{
			"test data",
			"wat",
			123,
			struct{ a int }{123},
		}

		sink luigi.FuncSink = func(ctx context.Context, v interface{}, err error) error {
			require.Less(t, calledCount, 2*len(testData), "call count mismatch")
			require.Equal(t, v, testData[calledCount%len(testData)], "test data mismatch")

			calledCount++
			return nil
		}
	)

	lr, err := NewLiveResumer(f, sink)
	require.NoError(t, err, "new live resumer")

	for i, v := range testData {
		t.Log(i, v)

		seq := margaret.BaseSeq(i)
		err = lr.Pour(context.TODO(), margaret.WrapWithSeq(v, seq))
		require.NoError(t, err, "pour")

		require.Equal(t, lr.seq, int64(seq))

		var (
			qs  = lr.QuerySpec()
			qry = &marmock.FakeQuery{}
		)

		err = qs(qry)
		require.NoError(t, err)

		require.Equal(t, qry.GtCallCount(), 1)
		require.Equal(t, qry.GtArgsForCall(0), margaret.BaseSeq(i))

		require.Equal(t, qry.GteCallCount(), 0)
		require.Equal(t, qry.LimitCallCount(), 0)
		require.Equal(t, qry.LiveCallCount(), 0)
		require.Equal(t, qry.LtCallCount(), 0)
		require.Equal(t, qry.LteCallCount(), 0)

		require.Equal(t, qry.SeqWrapCallCount(), 1)
		require.Equal(t, qry.SeqWrapArgsForCall(0), true)
	}

	lr, err = NewLiveResumer(f, sink)
	require.NoError(t, err, "new live resumer")

	for j, v := range testData {
		i := j + 4
		t.Log(i, v)

		seq := margaret.BaseSeq(i)
		err = lr.Pour(context.TODO(), margaret.WrapWithSeq(v, seq))
		require.NoError(t, err, "pour")

		require.Equal(t, lr.seq, int64(seq))

		var (
			qs  = lr.QuerySpec()
			qry = &marmock.FakeQuery{}
		)

		err = qs(qry)
		require.NoError(t, err)

		require.Equal(t, qry.GtCallCount(), 1)
		require.Equal(t, qry.GtArgsForCall(0), margaret.BaseSeq(i))

		require.Equal(t, qry.GteCallCount(), 0)
		require.Equal(t, qry.LimitCallCount(), 0)
		require.Equal(t, qry.LiveCallCount(), 0)
		require.Equal(t, qry.LtCallCount(), 0)
		require.Equal(t, qry.LteCallCount(), 0)

		require.Equal(t, qry.SeqWrapCallCount(), 1)
		require.Equal(t, qry.SeqWrapArgsForCall(0), true)
	}

}
