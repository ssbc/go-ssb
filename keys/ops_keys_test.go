package keys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type opKeysDecode struct {
	Data []byte

	ExpKeys Keys
	ExpErr string
}

func (op opKeysDecode) Do(t *testing.T, env interface{}) {
	ks := &Keys{}

	_, err := ks.Write(op.Data)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on ks.Decode")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q on keys decode, get %v", op.ExpErr, err)
	}
}

type opKeysLen struct {
	Keys Keys
	ExpLen int
}

func (op opKeysLen) Do(t *testing.T, env interface{}) {
	l := op.Keys.Len()
	require.Equal(t, op.ExpLen, l, "wrong keys length")
}
