package processing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

func TestIndex(t *testing.T) {
	type testcase struct {
		name   string
		ins    []margaret.SeqWrapper
		nProcs int
	}

	var (
		mkTest = func(tc testcase) func(*testing.T) {
			mkMsgProcr := func(closed chan struct{}) MessageProcessor {
				var i int
				return mockMessageProcessor{
					procMsgFunc: func(ctx context.Context, msg ssb.Message, seq margaret.Seq) error {
						require.Greater(t, len(tc.ins), i)

						var (
							in    = tc.ins[i]
							seqIn = in.Seq()
							msgIn = in.Value().(ssb.Message)
						)

						require.Equal(t, seqIn, seq, "seq")
						require.Equal(t, msgIn, msg, "msg")

						i += 1

						return nil
					},
					closeFunc: func(ctx context.Context) error {
						close(closed)
						return nil
					},
				}
			}
			return func(t *testing.T) {
				t.Parallel()

				var (
					procs  = make([]MessageProcessor, tc.nProcs)
					closes = make([]chan struct{}, tc.nProcs)
					ctx    = context.Background()
				)

				for i := range procs {
					closes[i] = make(chan struct{})
					procs[i] = mkMsgProcr(closes[i])
				}

				idx := &Index{
					Procs: procs,
				}

				for i, in := range tc.ins {
					err := idx.Pour(ctx, in)
					require.NoErrorf(t, err, "pour, %d", i)
				}

				err := idx.Close()
				require.NoError(t, err, "close")

				for i, closed := range closes {
					select {
					case <-closed:
					default:
						t.Errorf("not closed: %d", i)
					}
				}
			}
		}

		tcs = []testcase{
			{
				name:   "basic",
				nProcs: 4,
				ins: []margaret.SeqWrapper{
					margaret.WrapWithSeq(&ssb.KeyValueRaw{Value: ssb.Value{Content: []byte{0}}}, margaret.BaseSeq(0)),
					margaret.WrapWithSeq(&ssb.KeyValueRaw{Value: ssb.Value{Content: []byte{1}}}, margaret.BaseSeq(1)),
					margaret.WrapWithSeq(&ssb.KeyValueRaw{Value: ssb.Value{Content: []byte{2}}}, margaret.BaseSeq(2)),
				},
			},
			{
				name:   "skipping",
				nProcs: 4,
				ins: []margaret.SeqWrapper{
					margaret.WrapWithSeq(&ssb.KeyValueRaw{Value: ssb.Value{Content: []byte{0}}}, margaret.BaseSeq(0)),
					margaret.WrapWithSeq(&ssb.KeyValueRaw{Value: ssb.Value{Content: []byte{1}}}, margaret.BaseSeq(1)),
					margaret.WrapWithSeq(&ssb.KeyValueRaw{Value: ssb.Value{Content: []byte{3}}}, margaret.BaseSeq(3)),
				},
			},
		}
	)

	for _, tc := range tcs {
		t.Run(tc.name, mkTest(tc))
	}
}
