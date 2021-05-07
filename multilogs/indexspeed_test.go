package multilogs

import (
	"context"
	"math/rand"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
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

func TestIndexFixture(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	shasum := exec.Command("bash", "./integration_prep.bash")
	out, err := shasum.CombinedOutput()
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

	tr := repo.New(filepath.Join("testrun", t.Name()))

	testLog, err := repo.OpenLog(tr)
	r.NoError(err, "case %s failed to open", t.Name())

	sv, err := testLog.Seq().Value()
	r.NoError(err)
	if !a.EqualValues(tc.Length, sv.(margaret.Seq).Seq()+1, "case %s has wrong number of messages", t.Name()) {
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

			sv, err := sublog.Seq().Value()
			r.NoError(err)
			sublogSeq := sv.(margaret.Seq).Seq()

			fr := sr.Feed().Ref()
			seq, has := tc.HeadCount[fr]
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

	mkvMlog, snk, err := repo.OpenMultiLog(tr, "testmkv"+tc.Name, UserFeedsUpdate)
	r.NoError(err)
	serve("mkv", snk)
	compare(mkvMlog)
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
		sv, err := testLog.Seq().Value()
		r.NoError(err)
		r.EqualValues(logLen, sv.(margaret.Seq).Seq()+1)

		var seqs []margaret.Seq
		for j := i; j > 0; j-- {
			seqs = append(seqs, margaret.BaseSeq(rand.Int63n(logLen)))

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
