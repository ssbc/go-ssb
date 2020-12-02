package luigiutils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
)

func TestSinkCounter(t *testing.T) {
	r := require.New(t)

	ctx := context.TODO()

	var cnt int

	var vals []interface{}

	snk := luigi.NewSliceSink(&vals)

	wrappedSnk := NewSinkCounter(&cnt, snk)

	err := wrappedSnk.Pour(ctx, 1)
	r.NoError(err)

	err = wrappedSnk.Pour(ctx, 2)
	r.NoError(err)

	err = wrappedSnk.Pour(ctx, 3)
	r.NoError(err)

	r.EqualValues(3, cnt)
	r.Len(vals, 3)
}
