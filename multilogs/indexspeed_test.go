// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package multilogs

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ssbc/go-luigi"
	"github.com/ssbc/margaret"
	librarian "github.com/ssbc/margaret/indexes"
	"github.com/ssbc/margaret/multilog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb-refs/tfk"
	"github.com/ssbc/go-ssb/repo"
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

	outputList := false
	f, err := os.Open("v3-sloop-authors.json")
	var feedsSloop tFeedSet
	if err == nil {
		err = json.NewDecoder(f).Decode(&feedsSloop)
		r.NoError(err)
		f.Close()
	} else {
		t.Log("log", t.Name(), "authors not found - this test will fail but will result in an authors file being generated")
		outputList = true
	}

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

	if outputList {
		// write out a list of authors and exit
		authors := make(map[string]int64)

		ml, combinedSnk, closer := setupCombinedIndex(t, testLog, makeFsMlog)
		serve("combined", combinedSnk)

		addrs, err := ml.List()
		r.NoError(err)

		for i, addr := range addrs {
			var sr tfk.Feed
			err := sr.UnmarshalBinary([]byte(addr))
			r.NoError(err, "ref %d invalid", i)

			sublog, err := ml.Get(addr)
			r.NoError(err)

			sublogSeq := sublog.Seq()

			fr, err := sr.Feed()
			r.NoError(err)

			authors[fr.String()] = sublogSeq
			log.Printf("setting %s to %d", fr.String(), sublogSeq)
		}
		log.Printf("writing authors")
		r.NoError(testLog.Close())
		f, err := os.OpenFile("v3-sloop-authors.json", os.O_WRONLY|os.O_CREATE, 0644)
		r.NoError(err)
		err = json.NewEncoder(f).Encode(authors)
		r.NoError(err)
		f.Close()
		r.NoError(closer.Close())
		return
	}

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
