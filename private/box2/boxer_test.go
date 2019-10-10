package box2

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoxer(t *testing.T) {
	bxr := &Boxer{rand: rand.New(rand.NewSource(161))}

	buf := make([]byte, 4096)
	key := make(Key, KeySize)

	phrase := "squeamish ossifrage"

	msg, err := bxr.Encrypt(buf, []byte(phrase), nil, []Key{key})
	require.NoError(t, err, "encrypt")

	ctxt := make([]byte, len(msg.Raw))
	copy(ctxt, msg.Raw)

	plain, err := bxr.Decrypt(buf, ctxt, nil, []Key{key})
	require.NoError(t, err, "decrypt")

	require.Equal(t, phrase, string(plain), "wrong words")
}
