package keys

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger/v3"
	"github.com/keks/testops"
	librarian "go.cryptoscope.co/margaret/indexes"
)

func TestStoreSubfeeds(t *testing.T) {
	tDir := filepath.Join("testrun", t.Name())
	os.RemoveAll(tDir)
	os.MkdirAll(tDir, 0700)

	var (
		idx librarian.SeqSetterIndex
		db  *badger.DB
		mgr Store

		seed = make([]byte, 32)
	)

	tcs := []testops.TestCase{
		{
			Name: "single subfeed",
			Ops: []testops.Op{
				opDBCreate{
					Name: filepath.Join(tDir, "testdb"),
					DB:   &db,
				},
				opIndexNew{
					DB:    &db,
					Type:  Keys(nil),
					Index: &idx,
				},
				opDo(func(t *testing.T, env interface{}) {
					mgr = Store{idx}
				}),
				opDo(func(t *testing.T, env interface{}) {
					rand.Read(seed)

				}),
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     ID("test"),
					Scheme: SchemeMetafeedSubKey,
					Key:    Key("topsecret")},
			},
		},
	}

	testops.Run(t, []testops.Env{
		{
			Name: "Subfeed-Keys",
			Func: func(tc testops.TestCase) (func(*testing.T), error) {
				return tc.Runner(nil), nil
			},
		},
	}, tcs)
}
