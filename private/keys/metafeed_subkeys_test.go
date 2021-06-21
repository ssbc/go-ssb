package keys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger/v3"
	"github.com/keks/testops"
	"github.com/ssb-ngi-pointer/go-metafeed/metakeys"
	"github.com/stretchr/testify/require"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

func XTestStoreSubfeeds(t *testing.T) {
	tDir := filepath.Join("testrun", t.Name())
	os.RemoveAll(tDir)
	os.MkdirAll(tDir, 0700)

	var (
		idx librarian.SeqSetterIndex
		db  *badger.DB
		mgr Store
	)

	seed, err := metakeys.GenerateSeed()
	require.NoError(t, err)

	kp, err := metakeys.DeriveFromSeed(seed, metakeys.RootLabel, refs.RefAlgoFeedBendyButt)
	if err != nil {
		t.Fatal(err)
	}
	metaID := ID(storedrefs.Feed(kp.Feed))
	metaSecret := Key(kp.PrivateKey)

	legacySubfeed, err := metakeys.DeriveFromSeed(seed, "subfeed:1", refs.RefAlgoFeedSSB1)
	if err != nil {
		t.Fatal(err)
	}
	subfeed1ID := ID(storedrefs.Feed(legacySubfeed.Feed))
	subfeed1Secret := Key(legacySubfeed.PrivateKey)

	ggSubfeed, err := metakeys.DeriveFromSeed(seed, "subfeed:2", refs.RefAlgoFeedGabby)
	if err != nil {
		t.Fatal(err)
	}
	subfeed2ID := ID(storedrefs.Feed(ggSubfeed.Feed))
	subfeed2Secret := Key(ggSubfeed.PrivateKey)

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

				// add and get the root metakey
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     metaID,
					Scheme: SchemeFeedMessageSigningKey,
					Key:    metaSecret,
				},
				opStoreGetKeys{
					Mgr:    &mgr,
					ID:     metaID,
					Scheme: SchemeFeedMessageSigningKey,
					ExpRecps: Recipients{
						Recipient{Key: metaSecret, Scheme: SchemeFeedMessageSigningKey},
					},
				},

				// add and get the first subkey
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     subfeed1ID,
					Scheme: SchemeFeedMessageSigningKey,
					Key:    subfeed1Secret,
				},
				opStoreGetKeys{
					Mgr:    &mgr,
					ID:     subfeed1ID,
					Scheme: SchemeFeedMessageSigningKey,
					ExpRecps: Recipients{
						Recipient{Key: subfeed1Secret, Scheme: SchemeFeedMessageSigningKey},
					},
				},

				// add the 1st subfeed to the meta key
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     metaID,
					Scheme: SchemeMetafeedSubkey,
					Key:    Key(subfeed1ID),
				},
				opStoreGetKeys{
					Mgr:    &mgr,
					ID:     metaID,
					Scheme: SchemeMetafeedSubkey,
					ExpRecps: Recipients{
						Recipient{Key: Key(subfeed1ID), Scheme: SchemeMetafeedSubkey},
					},
				},

				// add and get the 2nd subkey
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     subfeed2ID,
					Scheme: SchemeFeedMessageSigningKey,
					Key:    subfeed2Secret,
				},
				opStoreGetKeys{
					Mgr:    &mgr,
					ID:     subfeed2ID,
					Scheme: SchemeFeedMessageSigningKey,
					ExpRecps: Recipients{
						Recipient{Key: subfeed2Secret, Scheme: SchemeFeedMessageSigningKey},
					},
				},

				// add the 2nd subfeed to the meta key
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     metaID,
					Scheme: SchemeMetafeedSubkey,
					Key:    Key(subfeed2ID),
				},
				opStoreGetKeys{
					Mgr:    &mgr,
					ID:     metaID,
					Scheme: SchemeMetafeedSubkey,
					ExpRecps: Recipients{
						Recipient{Key: Key(subfeed1ID), Scheme: SchemeMetafeedSubkey},
						Recipient{Key: Key(subfeed2ID), Scheme: SchemeMetafeedSubkey},
					},
				},
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
