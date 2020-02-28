package processing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
)

func TestPrefixLogGet(t *testing.T) {
	type testcase struct {
		name   string
		prefix string
		addr   librarian.Addr
		output librarian.Addr
	}

	var (
		mkTest = func(tc testcase) func(*testing.T) {
			return func(t *testing.T) {
				t.Parallel()

				var (
					output librarian.Addr
					called bool
					mlog   = PrefixMultilog{
						mlog: mockMultilog{
							getFunc: func(addr librarian.Addr) (margaret.Log, error) {
								called = true
								output = addr
								return nil, nil
							},
						},
						prefix: tc.prefix,
					}
				)

				mlog.Get(tc.addr)
				require.Equal(t, true, called)
				require.Equal(t, tc.output, output)
			}
		}

		tcs = []testcase{
			{
				name:   "simple",
				prefix: "testprefix",
				addr:   "testAddr",
				output: librarian.Addr([]byte{
					0, 2,
					0, 10, 't', 'e', 's', 't', 'p', 'r', 'e', 'f', 'i', 'x',
					0, 8, 't', 'e', 's', 't', 'A', 'd', 'd', 'r',
				}),
			},
		}
	)

	for _, tc := range tcs {
		t.Run(tc.name, mkTest(tc))
	}

}

func TestPrefixLogList(t *testing.T) {
	type testcase struct {
		name   string
		prefix string
		addrs  []librarian.Addr
		output []librarian.Addr
	}

	var (
		mkTest = func(tc testcase) func(*testing.T) {
			return func(t *testing.T) {
				t.Parallel()

				var (
					output []librarian.Addr
					mlog   = PrefixMultilog{
						mlog: mockMultilog{
							listFunc: func() ([]librarian.Addr, error) {
								return tc.addrs, nil
							},
						},
						prefix: tc.prefix,
					}
				)

				output, err := mlog.List()
				require.NoError(t, err)
				require.Equal(t, tc.output, output)
			}
		}

		tcs = []testcase{
			{
				name:   "simple",
				prefix: "testprefix",
				addrs:  []librarian.Addr{"testAddr1", "testAddr2"},
				output: []librarian.Addr{
					librarian.Addr([]byte{
						0, 2,
						0, 10, 't', 'e', 's', 't', 'p', 'r', 'e', 'f', 'i', 'x',
						0, 9, 't', 'e', 's', 't', 'A', 'd', 'd', 'r', '1',
					}),
					librarian.Addr([]byte{
						0, 2,
						0, 10, 't', 'e', 's', 't', 'p', 'r', 'e', 'f', 'i', 'x',
						0, 9, 't', 'e', 's', 't', 'A', 'd', 'd', 'r', '2',
					}),
				},
			},
		}
	)

	for _, tc := range tcs {
		t.Run(tc.name, mkTest(tc))
	}
}

// TODO: Test PrefixMultilog.Delete (like .Get) and .Close (trivial)
