package netwraputil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func TestSpoof(t *testing.T) {
	r := require.New(t)

	rc, wc := net.Pipe()

	kp, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	wrap := SpoofRemoteAddress(kp.Id.PubKey())

	wrapped, err := wrap(wc)
	r.NoError(err)

	ref, err := ssb.GetFeedRefFromAddr(wrapped.RemoteAddr())
	r.NoError(err)
	r.True(ref.Equal(kp.Id))

	wc.Close()
	rc.Close()
}
