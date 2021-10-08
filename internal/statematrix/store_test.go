// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package statematrix

import (
	"bytes"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func TestNew(t *testing.T) {
	r := require.New(t)
	os.RemoveAll("testrun")
	os.Mkdir("testrun", 0700)
	m, err := New("testrun/new", testFeed(0))
	r.NoError(err)

	format := refs.RefAlgoFeedSSB1

	feeds := []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 5}},
		{Feed: testFeed(2), Note: ssb.Note{Replicate: true, Receive: true, Seq: 499}},
		{Feed: testFeed(5), Note: ssb.Note{Replicate: true, Receive: true, Seq: 3000}},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	start := time.Now()
	feeds = []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 2}},
		{Feed: testFeed(2), Note: ssb.Note{Replicate: true, Receive: true, Seq: 20}},
		{Feed: testFeed(3), Note: ssb.Note{Replicate: true, Receive: true, Seq: 200}},
		{Feed: testFeed(4), Note: ssb.Note{Replicate: true, Receive: true, Seq: 2000}},
	}
	r.NoError(m.Fill(testFeed(23), feeds))
	t.Logf("%v (took: %s)", m, time.Since(start))

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 4}},
		{Feed: testFeed(2), Note: ssb.Note{Replicate: true, Receive: true, Seq: 500}},
		{Feed: testFeed(5), Note: ssb.Note{Replicate: true, Receive: true, Seq: 9000}},
	}
	r.NoError(m.Fill(testFeed(23), feeds))

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 3}},
		{Feed: testFeed(2), Note: ssb.Note{Replicate: true, Receive: true, Seq: 499}},
		{Feed: testFeed(5), Note: ssb.Note{Replicate: true, Receive: true, Seq: 1000}},
	}
	r.NoError(m.Fill(testFeed(13), feeds))

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 5}},
		{Feed: testFeed(2), Note: ssb.Note{Replicate: true, Receive: true, Seq: 750}},
		{Feed: testFeed(5), Note: ssb.Note{Replicate: true, Receive: true, Seq: 1000}},
	}
	r.NoError(m.Fill(testFeed(9), feeds))

	has, err := m.HasLonger(format)
	r.NoError(err)
	r.Len(has, 3)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: testFeed(2), Note: ssb.Note{Replicate: true, Receive: true, Seq: 750}},
		{Feed: testFeed(5), Note: ssb.Note{Replicate: true, Receive: true, Seq: 1000}},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger(format)
	r.NoError(err)
	r.Len(has, 1)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: testFeed(5), Note: ssb.Note{Replicate: true, Receive: true, Seq: 9000}},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger(format)
	r.NoError(err)
	r.Len(has, 0)

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 0}},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger(format)
	r.NoError(err)
	r.Len(has, 3)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 10}},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger(format)
	r.NoError(err)
	r.Len(has, 0)

	r.NoError(m.Close())
}

func testFeed(i int) refs.FeedRef {
	k := bytes.Repeat([]byte(strconv.Itoa(i)), 32)
	if len(k) > 32 {
		k = k[:32]
	}

	ref, err := refs.NewFeedRefFromBytes(k, refs.RefAlgoFeedSSB1)
	if err != nil {
		panic(err)
	}

	return ref
}

func TestChanged(t *testing.T) {
	r := require.New(t)
	os.RemoveAll("testrun")
	os.Mkdir("testrun", 0700)
	m, err := New("testrun/new", testFeed(0))
	r.NoError(err)

	// 0 has seen feed(1) up until 2
	feeds := []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 2}},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	// feed(1) already has 25 tho
	feeds = []ObservedFeed{
		{Feed: testFeed(1), Note: ssb.Note{Replicate: true, Receive: true, Seq: 25}},
	}
	r.NoError(m.Fill(testFeed(1), feeds))

	changed, err := m.Changed(testFeed(0), testFeed(1), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// changed should have 1 as two still (to get just 3)
	note, has := changed.Frontier[testFeed(1).String()]
	r.True(has, "changed doesnt have feed(1) (has %d entries)", len(changed.Frontier))
	r.Equal(int64(2), note.Seq)
	r.True(note.Replicate)
	r.True(note.Receive)
}
