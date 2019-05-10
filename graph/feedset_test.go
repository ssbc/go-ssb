package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
)

func TestFeedSetCount(t *testing.T) {
	r := require.New(t)
	kps := make([]*ssb.KeyPair, 50)

	fs := NewFeedSet(50)
	for i := 0; i < 50; i++ {
		var err error
		kps[i], err = ssb.NewKeyPair(nil)
		r.NoError(err)
		err = fs.AddRef(kps[i].Id)
		r.NoError(err)
	}
	r.Equal(50, fs.Count())
	lst, err := fs.List()
	r.NoError(err)
	r.Len(lst, 50, "first len(List()) wrong")
	// twice
	for i := 0; i < 50; i++ {
		err := fs.AddRef(kps[i].Id)
		r.NoError(err)
	}
	r.Equal(50, fs.Count())
	lst, err = fs.List()
	r.NoError(err)
	r.Len(lst, 50, "twice len(List()) wrong")
	// some
	for i := 0; i < 15; i++ {
		err := fs.AddRef(kps[i].Id)
		r.NoError(err)
	}
	r.Equal(50, fs.Count())
	lst, err = fs.List()
	r.NoError(err)
	r.Len(lst, 50, "some len(List()) wrong")
}
