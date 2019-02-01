package ssb

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

var TestKeyPair KeyPair

func init() {
	publicKeyStr := "VJM7w1W19ZsKmG2KnfaoKIM66BRoreEkzaVm/J//wl8="
	copy(TestKeyPair.Pair.Public[:], []byte(publicKeyStr))
}

func TestnewAdvertisement(t *testing.T) {
	tests := []struct {
		local       net.Addr
		keyPair     KeyPair
		Expected    string
		ExpectError bool
	}{}

	for _, test := range tests {
		res, err := newAdvertisement(test.local, &test.keyPair)
		if test.ExpectError {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, test.Expected, res)
	}
}
