package multilogs

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/indexes"
	idxbadger "go.cryptoscope.co/margaret/indexes/badger"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/margaret/multilog/roaring"
	multifs "go.cryptoscope.co/margaret/multilog/roaring/fs"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/multicloser"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/private/keys"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

func BenchmarkIndexFixturesCombined(b *testing.B) {
	r := require.New(b)

	testPath := filepath.Join("testrun", b.Name())

	fetchFixture := exec.Command("bash", "./integration_prep.bash", filepath.Join(testPath, "log"))
	out, err := fetchFixture.CombinedOutput()
	if err != nil {
		b.Log(string(out))
		r.NoError(err)
	}

	tr := repo.New(testPath)

	testLog, err := repo.OpenLog(tr)
	r.NoError(err, "case %s failed to open", b.Name())

	sv, err := testLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(100000, sv.(margaret.Seq).Seq()+1, "testLog has wrong number of messages")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {

		b.StopTimer()
		_, snk, closer := setupCombinedIndex(b, testLog, makeFsMlog)
		r.NoError(err)
		b.StartTimer()

		src, err := testLog.Query(snk.QuerySpec())
		r.NoError(err)

		err = luigi.Pump(context.TODO(), snk, src)
		r.NoError(err)
		b.StopTimer()
		closer.Close()
		os.RemoveAll(filepath.Join(testPath, "combinedIndexes"))
	}

}

func setupCombinedIndex(t testing.TB, rxlog margaret.Log, mkMlog makeMultilog) (multilog.MultiLog, indexes.SinkIndex, io.Closer) {
	r := require.New(t)
	testPath := filepath.Join("testrun", t.Name(), "combinedIndexes")
	testRepo := repo.New(testPath)

	keysDB, err := repo.OpenBadgerDB(testPath)
	r.NoError(err, "openIndex: failed to open keys database")

	idxKeys := idxbadger.NewIndex(keysDB, keys.Recipients{})

	ks := &keys.Store{
		Index: idxKeys,
	}

	var (
		tp testPublisher
		tg testGetter
	)

	tkp, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	sm, err := statematrix.New(
		testRepo.GetPath("ebt-state-matrix"),
		tkp.ID(),
	)
	r.NoError(err)

	var mc multicloser.MultiCloser

	tangles := mkMlog(t, testRepo, "tangles", &mc)

	gm := private.NewManager(tkp, tp, ks, rxlog, tg, tangles)

	user := mkMlog(t, testRepo, "user", &mc)
	private := mkMlog(t, testRepo, "private", &mc)
	byType := mkMlog(t, testRepo, "byType", &mc)
	groupMembers := mkMlog(t, testRepo, "groupMembers", &mc)

	snk, err := NewCombinedIndex(filepath.Join(testPath, "combined"),
		gm,
		tkp.ID(),
		rxlog,

		user,
		private,
		byType,
		tangles,
		groupMembers,

		sm,
	)
	if err != nil {
		t.Fatal(err)
	}
	mc.AddCloser(snk)

	return user, snk, &mc
}

func makeFsMlog(t testing.TB, r repo.Interface, name string, mc *multicloser.MultiCloser) *roaring.MultiLog {
	ml, err := multifs.NewMultiLog(r.GetPath("mlog", name))
	if err != nil {
		t.Fatalf("failed to open mlog: %s: %s", name, err)
	}
	mc.AddCloser(ml)
	return ml
}

type makeMultilog func(t testing.TB, r repo.Interface, name string, mc *multicloser.MultiCloser) *roaring.MultiLog

type testPublisher struct{}

func (tp testPublisher) Get(_ margaret.Seq) (interface{}, error) {
	return nil, fmt.Errorf("cant get from test publisher (just a stub)")
}

func (tp testPublisher) Append(_ interface{}) (margaret.Seq, error) {
	return nil, fmt.Errorf("cant append in test setting")
}

func (tp testPublisher) Publish(_ interface{}) (refs.MessageRef, error) {
	return refs.MessageRef{}, fmt.Errorf("cant publish in test setting")
}

func (tp testPublisher) Seq() luigi.Observable {
	panic("not implemented") // TODO: Implement
}

type testGetter struct{}

func (tg testGetter) Get(_ refs.MessageRef) (refs.Message, error) {
	panic("not implemented") // TODO: Implement
}

// Query returns a stream that is constrained by the passed query specification
func (tp testPublisher) Query(_ ...margaret.QuerySpec) (luigi.Source, error) {
	panic("not implemented") // TODO: Implement
}
