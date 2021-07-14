package luigiutils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/mem"
)

func TestJoinSources(t *testing.T) {
	r := require.New(t)

	la := mem.New()
	lb := mem.New()
	lc := mem.New()

	la.Append(1)
	la.Append(2)
	la.Append(3)

	srcA, err := la.Query()
	r.NoError(err)

	lb.Append("a")
	lb.Append("b")
	lb.Append("c")

	srcB, err := lb.Query()
	r.NoError(err)

	lc.Append(true)
	lc.Append(false)

	srcC, err := lc.Query()
	r.NoError(err)

	joinedSrc := JoinSources(srcA, srcB, srcC)

	ctx := context.TODO()
	i := 0
	for {
		v, err := joinedSrc.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			r.NoError(err)
		}
		t.Logf("%d: %v", i, v)
		i++
	}

	r.Equal(8, i)
}

func TestJoinSourcesLive(t *testing.T) {
	r := require.New(t)

	la := mem.New()
	lb := mem.New()

	valA := 1
	la.Append(valA)

	valB := byte('a')
	lb.Append(string(valB))

	srcA, err := la.Query(margaret.Live(true))
	r.NoError(err)

	srcB, err := lb.Query(margaret.Live(true))
	r.NoError(err)

	joinedSrc := JoinSources(srcA, srcB)

	ctx := context.TODO()
	i := 0
	for {
		v, err := joinedSrc.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			r.NoError(err)
		}

		// append new values each turn
		if i%2 == 0 {
			valA++
			la.Append(valA)
		} else {
			valB = byte(valB) + 1
			lb.Append(string(valB))
		}

		t.Logf("%2d: %v", i, v)

		i++

		if i > 10 {
			break
		}
	}

	r.Equal(11, i)
}
