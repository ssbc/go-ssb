package keys

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/keks/testops"
	"github.com/stretchr/testify/require"
	"modernc.org/kv"
)

/*
TODO: op-ify this.

possible ops:

db:
- create
- open
- close
- set 
- get
- put

manager:
- create
- add key
- set key
- rm key
- query keys

keys:
- decode
- len

dbkey:
- encode
- len
*/

func TestManagerOPd(t *testing.T) {
	tDir, err := ioutil.TempDir(".", "test-manager-")
	require.NoError(t, err, "mk temp dir")

	defer func() {
		require.NoError(t, os.RemoveAll(tDir), "rm temp dir")
	}()

	var (
		db *kv.DB
		mgr = &manager{db}
	)

	tcs := []testops.TestCase{
		testops.TestCase{
			Name: "compound test", // TODO: split this into smaller tests
			Ops: []testops.Op{
				opDBCreate{
					Name: "testdb",
					Opts: &kv.Options{},
					DB: &db,
				},
				opManagerAddKey{
					Mgr: mgr,
					ID: ID("test"),
					Key: Key("topsecret"),
				},
				opDBGet{
					DB: db,
					Key: []byte{
						4, 0, // db key is four byte long
						't','e','s','t', // "test"
					},

					ExpValue: []byte{
						1,0, // one key
						9,0, // key is 9 bytes long
						't','o','p','s','e','c','r','e','t', // "topsecret"
					},
				},
				opManagerAddKey{
					Mgr: mgr,
					ID: ID("test"),
					Key: Key("alsosecret"),
				},
				opDBGet{
					DB: db,
					Key: []byte{
						4, 0, // db key is four byte long
						't','e','s','t', // "test"
					},

					ExpValue: []byte{
						2,0, // two keys
						9,0, // first key is 9 bytes long
						't','o','p','s','e','c','r','e','t', // "topsecret"
						10,0, // second key is 10 bytes long
						'a', 'l','s','o','s','e','c','r','e','t', // "alsosecret"
					},
				},
				opManagerSetKey{
					Mgr: mgr,
					ID: ID("foo"),
					Key: Key("bar"),
				},
				opDBGet{
					DB: db,
					Key: []byte{
						3, 0, // db key is four byte long
						'f','o','o', // "foo"
					},

					ExpValue: []byte{
						1,0, // one key
						3,0, // key is 3 bytes long
						'b', 'a', 'r', // "bar"
					},
				},
				opManagerGetKeys{
					Mgr: mgr,
					ID: ID("test"),
					ExpKeys: Keys{Key("topsecret"), Key("alsosecret")},
				},
				opManagerGetKeys{
					Mgr: mgr,
					ID: ID("foo"),
					ExpKeys: Keys{Key("bar")},
				},
				opManagerRmKeys{
					ID: ID("test"),
				},
				opManagerGetKeys{
					ID: ID("test"),
					ExpKeys: Keys{},
				},
			},
		},
		{
			Name: "encode dbKey",
			Ops: []testops.Op{
				opDBKeyEncode{
					Key: &dbKey{
						t: 0,
						id: ID("test"),
					},
					ExpData: []byte{
						0, 0,
						4, 0,
						't', 'e', 's', 't',
					},
				},
				opDBKeyEncode{
					Key: &dbKey{
						t: 1,
						id: ID("test"),
					},
					ExpData: []byte{
						1, 0,
						4, 0,
						't', 'e', 's', 't',
					},
				},
			},
		},
	}

	testops.Run(t, nil, tcs)
}

func TestManager(t *testing.T) {
	tDir, err := ioutil.TempDir(".", "test-manager-")
	require.NoError(t, err, "mk temp dir")

	defer func() {
		require.NoError(t, os.RemoveAll(tDir), "rm temp dir")
	}()

	db, err := kv.Create(filepath.Join(tDir, "db"), &kv.Options{})
	require.NoError(t, err, "db open")

	mgr := &manager{db}

	err = mgr.AddKey(0, ID("test"), Key("topsecret"))
	require.NoError(t, err, "db add key 1")

	data, err := db.Get(nil, []byte{0,0,4,0,'t','e','s','t'})
	require.NoError(t, err, "db get")
	require.NotNil(t, data, "key is nil")
	require.Equal(t, []byte{1,0,9,0,'t','o','p','s','e','c','r','e','t'}, data, "key is not correct")

	err = mgr.AddKey(0, ID("test"), Key("alsosecret"))
	require.NoError(t, err, "mgr add key 2")

	data, err = db.Get(nil, []byte{0,0,4,0,'t','e','s','t'})
	require.NoError(t, err, "db get")
	require.NotNil(t, data, "key is nil")
	require.Equal(t, []byte{2,0,9,0,'t','o','p','s','e','c','r','e','t',10,0,'a','l','s','o','s','e','c','r','e','t'}, data, "key is not correct")

	err = mgr.SetKey(0, ID("foo"), Key("bar"))
	require.NoError(t, err, "mgr set foo")

	data, err = db.Get(nil, []byte{0,0,3,0,'f','o','o'})
	require.NoError(t, err, "db set foo")
	require.NotNil(t, data, "key is nil")
	require.Equal(t, []byte{1,0,3,0,'b','a','r'}, data, "key is not correct")

	ks, err := mgr.GetKeys(0, ID("foo"))
	require.NoError(t, err, "mgr get foo")
	require.Equal(t, &Keys{Key("bar")}, ks, "keys at foo")

	ks, err = mgr.GetKeys(0, ID("test"))
	require.NoError(t, err, "mgr get test")
	require.Equal(t, &Keys{Key("topsecret"), Key("alsosecret")}, ks, "keys at test")

	err = mgr.RmKeys(0, ID("test"))
	require.NoError(t, err, "mgr rm test")

	ks, err = mgr.GetKeys(0, ID("test"))
	require.Error(t, err, "mgr get test after rm")
}
