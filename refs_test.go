package ssb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRef(t *testing.T) {
	var tcases = []struct {
		ref  string
		err  error
		want Ref
	}{
		{"xxxx", ErrInvalidRef, nil},
		{"+xxx.foo", ErrInvalidHash, nil},
		{"@xxx.foo", ErrInvalidHash, nil},
		{"&YWJj.sha256", nil, &BlobRef{[]byte("abc"), RefAlgoSHA256}},
		{"%YWJj.sha256", nil, &MessageRef{[]byte("abc"), RefAlgoSHA256}},
		{"@YWJj.ed25519", nil, &FeedRef{[]byte("abc"), RefAlgoEd25519}},
	}
	for i, tc := range tcases {
		r, err := ParseRef(tc.ref)
		if err != tc.err {
			t.Fatalf("%03d: Expected err:%v got:%v", i, tc.err, err)
		} else if tc.err == nil {
			assert.Equal(t, tc.want.Ref(), r.Ref(), "test %d failed", i)
		}
	}
}
