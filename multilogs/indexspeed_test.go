// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package multilogs

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

type tFeedSet map[string]int
type tcase struct {
	Name      string
	Length    int64
	HeadCount tFeedSet
}

func BenchmarkIndexFixturesUserFeeds(b *testing.B) {
	r := require.New(b)

	testRepo := filepath.Join("testrun", b.Name())

	fetchFixture := exec.Command("bash", "./integration_prep.bash", filepath.Join(testRepo, "log"))
	out, err := fetchFixture.CombinedOutput()
	if err != nil {
		b.Log(string(out))
		r.NoError(err)
	}

	tr := repo.New(filepath.Join("testrun", b.Name()))

	testLog, err := repo.OpenLog(tr)
	r.NoError(err, "case %s failed to open", b.Name())

	r.EqualValues(100000, testLog.Seq()+1, "testLog has wrong number of messages")

	b.ResetTimer()
	name := "benchidx"

	for n := 0; n < b.N; n++ {

		b.StopTimer()
		_, snk, err := repo.OpenFileSystemMultiLog(tr, name, UserFeedsUpdate) // fs-bitmaps
		r.NoError(err)
		b.StartTimer()

		src, err := testLog.Query(snk.QuerySpec())
		r.NoError(err)

		err = luigi.Pump(context.TODO(), snk, src)
		r.NoError(err)
		b.StopTimer()
		snk.Close()
		os.RemoveAll(tr.GetPath(repo.PrefixMultiLog, name))
	}
}

func TestIndexFixtures(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	if testutils.SkipOnCI(t) {
		return
	}

	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
		// TODO: expose index flushing
	}

	f, err := os.Open("v2-sloop-authors.json")
	r.NoError(err)
	var feedsSloop tFeedSet
	err = json.NewDecoder(f).Decode(&feedsSloop)
	r.NoError(err)
	f.Close()

	testRepo := filepath.Join("testrun", t.Name())

	fetchFixture := exec.Command("bash", "./integration_prep.bash", filepath.Join(testRepo, "log"))
	out, err := fetchFixture.CombinedOutput()
	if err != nil {
		t.Log(string(out))
		r.NoError(err)
	}

	mediumCase := tcase{
		Name:      "fixtures-sloop",
		Length:    100000,
		HeadCount: feedsSloop,
	}
	tc := mediumCase

	tr := repo.New(testRepo)

	testLog, err := repo.OpenLog(tr)
	r.NoError(err, "case %s failed to open", t.Name())

	if !a.EqualValues(tc.Length, testLog.Seq()+1, "case %s has wrong number of messages", t.Name()) {
		return
	}

	// helper functions

	serve := func(idx string, snk librarian.SinkIndex) {
		src, err := testLog.Query(snk.QuerySpec())
		r.NoError(err)

		start := time.Now()
		err = luigi.Pump(context.TODO(), snk, src)
		r.NoError(err)
		t.Log("log", t.Name(), "index", idx, "took", time.Since(start))
	}

	// f, err := os.Create("/tmp/lengthfile")
	// r.NoError(err)

	compare := func(ml multilog.MultiLog) {
		addrs, err := ml.List()
		r.NoError(err)
		a.Equal(len(tc.HeadCount), len(addrs))

		for i, addr := range addrs {
			var sr tfk.Feed
			err := sr.UnmarshalBinary([]byte(addr))
			r.NoError(err, "ref %d invalid", i)

			sublog, err := ml.Get(addr)
			r.NoError(err)

			sublogSeq := sublog.Seq()

			fr, err := sr.Feed()
			r.NoError(err)

			seq, has := tc.HeadCount[fr.String()]
			if !a.True(has, "feed not found:%s", fr) {
				// fmt.Fprintf(f, "%q:%d,\n", fr, sublogSeq)
				continue
			}

			if a.True(has, "ref %s not in set (has:%d)", fr, sublogSeq) {
				a.EqualValues(seq, sublogSeq,
					"%s: sublog %s has wrong number of messages", tc.Name, fr)
			}
		}
		r.NoError(ml.Close())
	}

	mkvMlog, snk, err := repo.OpenStandaloneMultiLog(tr, "testbadger"+tc.Name, UserFeedsUpdate)
	r.NoError(err)
	serve("badger", snk)
	compare(mkvMlog)

	fsMlog, snk, err := repo.OpenFileSystemMultiLog(tr, "testfs"+tc.Name, UserFeedsUpdate)
	r.NoError(err)
	serve("fs-bitmap", snk)
	compare(fsMlog)

	userMlog, combinedSnk, closer := setupCombinedIndex(t, testLog, makeFsMlog)
	serve("combined", combinedSnk)
	compare(userMlog)
	r.NoError(closer.Close())
}

func benchSequential(i int) func(b *testing.B) {
	return func(b *testing.B) {
		r := require.New(b)
		ctx := context.TODO()

		testRepo := repo.New(filepath.Join("testrun", "TestIndexFixture"))
		testLog, err := repo.OpenLog(testRepo)
		r.NoError(err)

		src, err := testLog.Query(margaret.Limit(i))
		r.NoError(err)

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
		readLoop:
			for {
				v, err := src.Next(ctx)
				if luigi.IsEOS(err) {
					break readLoop
				} else {
					r.NoError(err)
				}
				_, ok := v.(refs.Message)
				r.True(ok)
			}
		}
	}
}

func BenchmarkReadSequential(b *testing.B) {
	b.Run("100", benchSequential(100))
	b.Run("5k", benchSequential(5000))
	b.Run("20k", benchSequential(20000))
}

func benchRandom(i int) func(b *testing.B) {
	return func(b *testing.B) {
		r := require.New(b)

		testRepo := repo.New(filepath.Join("testrun", "TestIndexFixture"))
		testLog, err := repo.OpenLog(testRepo)
		r.NoError(err)

		logLen := int64(100000)
		r.EqualValues(logLen, testLog.Seq()+1)

		var seqs []int64
		for j := i; j > 0; j-- {
			seqs = append(seqs, rand.Int63n(logLen))

		}

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for _, seq := range seqs {
				v, err := testLog.Get(seq)
				r.NoError(err)
				_, ok := v.(refs.Message)
				r.True(ok)
				// r.NotNil(msg.Key())
				// r.NotNil(msg.Author())
			}

		}
	}
}

func BenchmarkRandom(b *testing.B) {
	b.Run("100", benchRandom(100))
	b.Run("5k", benchRandom(5000))
	b.Run("20k", benchRandom(20000))
}
