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

		{"%wayTooShort.sha256", ErrInvalidHash, nil},
		{"&tooShort.sha256", NewHashLenError(6), nil},
		{"@tooShort.ed25519", NewFeedRefLenError(6), nil},
		{"&c29tZU5vbmVTZW5zZQo=.sha256", NewHashLenError(14), nil},
	}
	for i, tc := range tcases {
		r, err := ParseRef(tc.ref)
		if err != tc.err {
			assert.EqualError(t, err, tc.err.Error(), "%d wrong error", i)
		} else if tc.err == nil {
			assert.Equal(t, tc.want.Ref(), r.Ref(), "test %d failed", i)
		}
	}
}
