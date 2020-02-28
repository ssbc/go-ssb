package processing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeString(t *testing.T) {
	type testcase struct {
		name string

		out []byte
		str string

		output []byte
	}

	var (
		mkTest = func(tc testcase) func(*testing.T) {
			return func(t *testing.T) {
				t.Parallel()
				output := encodeString(tc.out, tc.str)
				require.Equal(t, tc.output, output)
			}
		}

		tcs = []testcase{
			{
				name:   "nil out slice",
				str:    "abc",
				output: []byte{0, 3, 'a', 'b', 'c'},
			},
			{
				name:   "preallocated out slice",
				out:    make([]byte, 0, 5),
				str:    "abc",
				output: []byte{0, 3, 'a', 'b', 'c'},
			},
			{
				name:   "prepopulated out slice",
				out:    []byte{'1', '2', '3', '4', '5', '6', '7'}[:0],
				str:    "abc",
				output: []byte{0, 3, 'a', 'b', 'c'},
			},
			{
				name:   "append to prepopulated out slice",
				out:    []byte("foo"),
				str:    "abc",
				output: []byte{'f', 'o', 'o', 0, 3, 'a', 'b', 'c'},
			},
		}
	)

	for _, tc := range tcs {
		t.Run(tc.name, mkTest(tc))
	}
}

func TestEncodeAppendStrings(t *testing.T) {
	type testcase struct {
		name string

		base    []byte
		strings []string

		output []byte
	}

	var (
		mkTest = func(tc testcase) func(*testing.T) {
			return func(t *testing.T) {
				t.Parallel()
				output := encodeAppendStrings(tc.base, tc.strings...)
				require.Equal(t, tc.output, output)
			}
		}

		tcs = []testcase{
			{
				name:    "simple case, empty base",
				strings: []string{"foo", "bar"},
				output:  []byte{0, 2, 0, 3, 'f', 'o', 'o', 0, 3, 'b', 'a', 'r'},
			},
			{
				name:    "simple case, prepopulated base",
				base:    []byte{0, 1, 0, 3, 'a', 'b', 'c'},
				strings: []string{"foo", "bar"},
				output:  []byte{0, 3, 0, 3, 'a', 'b', 'c', 0, 3, 'f', 'o', 'o', 0, 3, 'b', 'a', 'r'},
			},
		}
	)

	for _, tc := range tcs {
		t.Run(tc.name, mkTest(tc))
	}
}
