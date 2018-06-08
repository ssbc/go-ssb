package ssb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify(t *testing.T) {
	a, r := assert.New(t), require.New(t)
	n := len(testMessages)
	if testing.Short() {
		n = min(50, n)
	}
	for i := 1; i < n; i++ {
		hash, _, err := Verify(testMessages[i].Input)
		r.NoError(err, "verify failed")
		a.Equal(testMessages[i].Hash, hash.Ref(), "hash mismatch %d", i)
	}
}
