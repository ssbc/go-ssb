package keys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDBKey(t *testing.T) {
	dbk := &dbKey{
		t: 0,
		id: ID("test"),
	}

	dbkBs, err := dbk.MarshalBinary()
	require.NoError(t, err, "dbk marshal")
	require.Equal(t, []byte{0,0,4,0,'t','e','s','t'}, dbkBs)
}
