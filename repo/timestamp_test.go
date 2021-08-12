// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package repo_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret/mem"
	"go.cryptoscope.co/ssb/repo"

	refs "go.mindeco.de/ssb-refs"
)

func TestTimestampSorting(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	rxlog := mem.New()
	var allSeqs []int64

	// make log with messages in reverse chronological order
	for i := 10000; i > 0; i -= 1000 {
		seq, err := rxlog.Append(mkTestMessage(i))
		r.NoError(err)
		t.Log(seq, i)
		allSeqs = append(allSeqs, int64(seq))
	}

	sorter, err := repo.NewSequenceResolverFromLog(rxlog)
	r.NoError(err, "failed to get sliced array")

	under9000 := func(v int64) bool {
		ok := v < 9000
		t.Log(v, ok)
		return ok
	}
	sorted, err := sorter.SortAndFilter(allSeqs, repo.SortByClaimed, under9000, true)
	r.NoError(err)
	a.Len(sorted, 8)
	a.EqualValues(2, sorted[0].Seq)
	t.Log(sorted)

	under3000 := func(v int64) bool {
		ok := v < 3000
		t.Log(v, ok)
		return ok
	}
	smaller, err := sorter.SortAndFilter(allSeqs, repo.SortByClaimed, under3000, false)
	r.NoError(err)
	a.Len(smaller, 2)
	a.EqualValues(9, smaller[0].Seq)
	t.Log(smaller)

	yes := func(_ int64) bool { return true }
	empty, err := sorter.SortAndFilter([]int64{10}, repo.SortByReceived, yes, false)
	r.Error(err)
	a.Nil(empty)

	// luigi.Source for backwards compat with the existing margaret code
	src := sorted.AsLuigiSource()

	var elems []interface{}
	snk := luigi.NewSliceSink(&elems)

	err = luigi.Pump(context.TODO(), snk, src)
	r.NoError(err)
	r.Len(elems, 8)
	t.Log(elems)
}

// utils

var seq int64

func mkTestMessage(ts int) refs.Message {
	sm := testMessage{
		seq:     seq,
		claimed: time.Unix(int64(ts), 0),
	}
	seq++
	return sm
}

type testMessage struct {
	seq     int64
	claimed time.Time
}

func (ts testMessage) Seq() int64 {
	return ts.seq
}

func (ts testMessage) Claimed() time.Time {
	return ts.claimed
}

func (ts testMessage) Received() time.Time {
	return time.Now()
}

func (ts testMessage) Key() refs.MessageRef {
	panic("not implemented")
}

func (ts testMessage) Previous() *refs.MessageRef {
	panic("not implemented")
}

func (ts testMessage) Author() refs.FeedRef {
	panic("not implemented")
}

func (ts testMessage) ContentBytes() []byte {
	panic("not implemented")
}

func (ts testMessage) ValueContent() *refs.Value {
	panic("not implemented")
}

func (ts testMessage) ValueContentJSON() json.RawMessage {
	panic("not implemented")
}
