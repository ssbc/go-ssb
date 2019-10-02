package keys

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"modernc.org/kv"
)

/*
TODO: op-ify this.

possible ops:
- create db
- open db
- close db
- set to db
- get from db
- put to db
- create manager
- add key
- set key
- rm key
- query keys

*/

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
