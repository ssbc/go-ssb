package statematrix

import (
	"bytes"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"
)

func TestNew(t *testing.T) {
	r := require.New(t)
	os.RemoveAll("testrun")
	os.Mkdir("testrun", 0700)
	m, err := New("testrun/new_test.ql")
	r.NoError(err)

	feeds := []ObservedFeed{
		{Feed: testFeed(1), Replicate: true, Receive: true, Len: 5},
		{Feed: testFeed(2), Replicate: true, Receive: true, Len: 499},
		{Feed: testFeed(5), Replicate: true, Receive: true, Len: 3000},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	start := time.Now()
	feeds = []ObservedFeed{
		{Feed: testFeed(1), Replicate: true, Receive: true, Len: 2},
		{Feed: testFeed(2), Replicate: true, Receive: true, Len: 20},
		{Feed: testFeed(3), Replicate: true, Receive: true, Len: 200},
		{Feed: testFeed(4), Replicate: true, Receive: true, Len: 2000},
	}
	r.NoError(m.Fill(testFeed(23), feeds))
	t.Log(m.String(), ", took:", time.Since(start))

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Replicate: true, Receive: true, Len: 4},
		{Feed: testFeed(2), Replicate: true, Receive: true, Len: 500},
		{Feed: testFeed(5), Replicate: true, Receive: true, Len: 9000},
	}
	r.NoError(m.Fill(testFeed(23), feeds))

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Replicate: true, Receive: true, Len: 3},
		{Feed: testFeed(2), Replicate: true, Receive: true, Len: 499},
		{Feed: testFeed(5), Replicate: true, Receive: true, Len: 1000},
	}
	r.NoError(m.Fill(testFeed(13), feeds))

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Replicate: true, Receive: true, Len: 5},
		{Feed: testFeed(2), Replicate: true, Receive: true, Len: 750},
		{Feed: testFeed(5), Replicate: true, Receive: true, Len: 1000},
	}
	r.NoError(m.Fill(testFeed(9), feeds))

	has, err := m.HasLonger()
	r.NoError(err)
	r.Len(has, 3)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: testFeed(2), Replicate: true, Receive: true, Len: 750},
		{Feed: testFeed(5), Replicate: true, Receive: true, Len: 1000},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 1)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: testFeed(5), Replicate: true, Receive: true, Len: 9000},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 0)

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Replicate: true, Receive: true, Len: 0},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 3)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: testFeed(1), Replicate: true, Receive: true, Len: 10},
	}
	r.NoError(m.Fill(testFeed(0), feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 0)

	r.NoError(m.Close())
}

func testFeed(i int) *refs.FeedRef {
	return &refs.FeedRef{
		Algo: "test",
		ID:   bytes.Repeat([]byte(strconv.Itoa(i)), 32),
	}
}
