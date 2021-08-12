// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package testutils

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"
)

func StreamLog(t *testing.T, l margaret.Log) {
	r := require.New(t)
	src, err := l.Query()
	r.NoError(err)
	i := 0
	for {
		v, err := src.Next(context.TODO())
		if luigi.IsEOS(err) {
			break
		}

		mm, ok := v.(refs.Message)
		r.True(ok, "%T", v)

		t.Logf("log seq: %d - %s:%d (%s)",
			i,
			mm.Author().ShortRef(),
			mm.Seq(),
			mm.Key().ShortRef())

		b := mm.ContentBytes()
		if n := len(b); n > 128 {
			t.Log("truncating", n, " to last 32 bytes")
			b = b[len(b)-32:]
		}
		t.Logf("\n%s", hex.Dump(b))

		i++
	}

}
