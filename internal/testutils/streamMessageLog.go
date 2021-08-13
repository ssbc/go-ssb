// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package testutils

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	refs "go.mindeco.de/ssb-refs"
)

func StreamLog(t *testing.T, l margaret.Log) {
	r := require.New(t)

	src, err := l.Query()
	r.NoError(err)

	seq := l.Seq()
	i := int64(0)

	for {
		v, err := src.Next(context.TODO())
		if luigi.IsEOS(err) {
			break
		}

		mm, ok := v.(refs.Message)
		r.True(ok, "expected %T to be a refs.Message (wrong log type? missing indirection to receive log?)", v)

		t.Logf("log seq: %d - %s:%d (%s)",
			i,
			mm.Author().ShortSigil(),
			mm.Seq(),
			mm.Key().ShortSigil())

		b := mm.ContentBytes()

		switch mm.Author().Algo() {
		case refs.RefAlgoFeedSSB1:
			const max = 512
			if n := len(b); n > max {
				t.Logf("truncating %d to last %d bytes", n, max)
				b = b[len(b)-max:]
			}
			t.Logf("\n%s", hex.Dump(b))

		case refs.RefAlgoFeedBendyButt:
			var v interface{}
			err = bencode.DecodeBytes(b, &v)
			if err != nil {
				continue
			}
			t.Logf("\n%s", spew.Sdump(v))
		}

		i++
	}

	// margaret is 0-indexed
	seq += 1
	if seq != i {
		t.Errorf("seq differs from iterated count: %d vs %d", seq, i)
	}
}
