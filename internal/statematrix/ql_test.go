package statematrix

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	r := require.New(t)
	os.RemoveAll("testrun")
	os.Mkdir("testrun", 0700)
	m, err := New("testrun/new_test.ql")
	r.NoError(err)

	feeds := []ObservedFeed{
		{Feed: 1, Replicate: true, Receive: true, Len: 5},
		{Feed: 2, Replicate: true, Receive: true, Len: 499},
		{Feed: 5, Replicate: true, Receive: true, Len: 3000},
	}
	r.NoError(m.Fill(0, feeds))

	start := time.Now()
	feeds = []ObservedFeed{
		{Feed: 1, Replicate: true, Receive: true, Len: 2},
		{Feed: 2, Replicate: true, Receive: true, Len: 20},
		{Feed: 3, Replicate: true, Receive: true, Len: 200},
		{Feed: 4, Replicate: true, Receive: true, Len: 2000},
	}
	r.NoError(m.Fill(23, feeds))
	t.Log(m.String(), ", took:", time.Since(start))

	feeds = []ObservedFeed{
		{Feed: 1, Replicate: true, Receive: true, Len: 4},
		{Feed: 2, Replicate: true, Receive: true, Len: 500},
		{Feed: 5, Replicate: true, Receive: true, Len: 9000},
	}
	r.NoError(m.Fill(23, feeds))

	feeds = []ObservedFeed{
		{Feed: 1, Replicate: true, Receive: true, Len: 3},
		{Feed: 2, Replicate: true, Receive: true, Len: 499},
		{Feed: 5, Replicate: true, Receive: true, Len: 1000},
	}
	r.NoError(m.Fill(13, feeds))

	feeds = []ObservedFeed{
		{Feed: 1, Replicate: true, Receive: true, Len: 5},
		{Feed: 2, Replicate: true, Receive: true, Len: 750},
		{Feed: 5, Replicate: true, Receive: true, Len: 1000},
	}
	r.NoError(m.Fill(9, feeds))

	has, err := m.HasLonger()
	r.NoError(err)
	r.Len(has, 3)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: 2, Replicate: true, Receive: true, Len: 750},
		{Feed: 5, Replicate: true, Receive: true, Len: 1000},
	}
	r.NoError(m.Fill(0, feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 1)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: 5, Replicate: true, Receive: true, Len: 9000},
	}
	r.NoError(m.Fill(0, feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 0)

	feeds = []ObservedFeed{
		{Feed: 1, Replicate: true, Receive: true, Len: 0},
	}
	r.NoError(m.Fill(0, feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 3)
	t.Logf("%+v", has)

	feeds = []ObservedFeed{
		{Feed: 1, Replicate: true, Receive: true, Len: 10},
	}
	r.NoError(m.Fill(0, feeds))

	has, err = m.HasLonger()
	r.NoError(err)
	r.Len(has, 0)

	r.NoError(m.Close())
}
