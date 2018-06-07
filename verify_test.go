package ssb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify(t *testing.T) {
	a, r := assert.New(t), require.New(t)
	n := min(20, len(testMessages))
	for i := 1; i < n; i++ {
		hash, _, err := Verify(testMessages[i].Input)
		r.NoError(err, "verify failed")
		a.Equal(hash.Ref(), testMessages[i].Hash)
	}
}
