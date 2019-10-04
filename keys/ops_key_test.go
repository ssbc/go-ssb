package keys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type opDBKeyEncode struct {
	Key *dbKey

	ExpData []byte
	ExpErr string
}

func (op opDBKeyEncode) Do(t *testing.T, env interface{}) {
	data, err := op.Key.MarshalBinary()
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on dbk.Encode")
	} else {
		require.EqualError(t, err, op.ExpErr, "wrong error")
	}
	require.Equal(t, op.ExpData, data, "wrong marshaled data")
}

type opDBKeyLen struct {
	Key *dbKey
	ExpLen int
}

func (op opDBKeyLen) Do(t *testing.T, env interface{}) {
	l := op.Key.Len()
	require.Equal(t, op.ExpLen, l, "wrong dbKey length")
}
