package processing

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/vartuple"
)

var enc vartuple.Encoding

func init() {
	enc.Binary = binary.BigEndian
}

func TestContentProcessor(t *testing.T) {
	type testcase struct {
		name string
		f    ContentProcessorFunc
		ins  []ssb.Message
		out  map[librarian.Addr][]margaret.Seq
	}

	var (
		mkTest = func(tc testcase) func(*testing.T) {
			return func(t *testing.T) {
				t.Parallel()

				var (
					// offs tracks how far we progressed in the individual
					// sublogs
					offs = make(map[librarian.Addr]int)

					// mkMockLog returns the mocked sublog for given addr
					mkMockLog = func(addr librarian.Addr) margaret.Log {
						return mockLog{
							appendFunc: func(v interface{}) (margaret.Seq, error) {
								var (
									logSlice = tc.out[addr]
									off      = offs[addr]
									seq      = margaret.BaseSeq(off)
								)
								require.Greater(t, len(logSlice), off, addr)
								require.Equal(t, logSlice[off], v)
								offs[addr] += 1
								return seq, nil
							},
						}
					}

					// cp is the ContentProcessor that uses the mocked multilog
					cp = ContentProcessor{
						F: tc.f,
						MLog: mockMultilog{
							getFunc: func(addr librarian.Addr) (margaret.Log, error) {
								return mkMockLog(addr), nil
							},
						},
					}
				)

				ctx := context.TODO()

				for i, in := range tc.ins {
					seq := margaret.BaseSeq(i)
					err := cp.ProcessMessage(ctx, in, seq)
					require.NoError(t, err)
				}

				for k := range offs {
					require.Equal(t, len(tc.out[k]), offs[k], k)
				}

				for k := range tc.out {
					require.Equal(t, len(tc.out[k]), offs[k], k)
				}
			}
		}

		msgFromContentString = func(content string) ssb.Message {
			return &ssb.KeyValueRaw{
				Value: ssb.Value{
					Content: []byte(content),
				},
			}
		}

		seqsFromInts = func(is ...int) []margaret.Seq {
			seqs := make([]margaret.Seq, 0, len(is))
			for _, i := range is {
				seqs = append(seqs, margaret.BaseSeq(i))
			}
			return seqs
		}

		tcs = []testcase{
			{
				name: "base case",
				f: func(content map[string]interface{}) ([]string, error) {
					var outs []string

					for k, v := range content {
						if str, ok := v.(string); ok {
							outs = append(outs, string(enc.Encode(nil, []byte(k), []byte(str))))
						}
					}

					return outs, nil
				},
				ins: []ssb.Message{
					msgFromContentString(`{"foo": "bar"}`),
					msgFromContentString(`{"bar": "baz"}`),
					msgFromContentString(`{"foo": "bar", "bar": "baz"}`),
				},
				out: map[librarian.Addr][]margaret.Seq{
					librarian.Addr(enc.Encode(nil, []byte("foo"), []byte("bar"))): seqsFromInts(0, 2),
					librarian.Addr(enc.Encode(nil, []byte("bar"), []byte("baz"))): seqsFromInts(1, 2),
				},
			},
		}
	)

	for _, tc := range tcs {
		t.Run(tc.name, mkTest(tc))
	}
}
