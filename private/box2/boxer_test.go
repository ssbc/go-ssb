package box2

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/ssb/keys"
)

func TestBoxer(t *testing.T) {
	bxr := &Boxer{rand: rand.New(rand.NewSource(161))}

	buf := make([]byte, 4096)
	key := make(keys.Key, KeySize)
	key2 := make(keys.Key, KeySize)

	bxr.rand.Read(key)
	bxr.rand.Read(key2)

	phrase := "squeamish ossifrage"

	msg, err := bxr.Encrypt(buf, []byte(phrase), nil, keys.Keys{key, key2})
	require.NoError(t, err, "encrypt")

	ctxt := make([]byte, len(msg.Raw))
	copy(ctxt, msg.Raw)

	plain, err := bxr.Decrypt(buf, ctxt, nil, keys.Keys{key})
	require.NoError(t, err, "decrypt")
	require.Equal(t, phrase, string(plain), "wrong words")

	plain, err = bxr.Decrypt(buf, ctxt, nil, keys.Keys{key2})
	require.NoError(t, err, "decrypt")
	require.Equal(t, phrase, string(plain), "wrong words")
}
