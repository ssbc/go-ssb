package keys

import (
	"testing"

	"modernc.org/kv"
	"github.com/stretchr/testify/require"
)

type opDBCreate struct {
	Name string
	Opts *kv.Options

	ExpErr string
	DB **kv.DB
}

func (op opDBCreate) Do(t *testing.T, env interface{}) {
	var err error
	*(op.DB), err = kv.Create(op.Name, op.Opts)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on kv.Create")
	} else {
		require.EqualError(t, err, op.ExpErr, "expected different error on kv.Create")
	}
}

type opDBOpen struct {
	Name string
	Opts *kv.Options

	ExpErr string
	DB **kv.DB
}

func (op opDBOpen) Do(t *testing.T, env interface{}) {
	var err error
	*(op.DB), err = kv.Open(op.Name, op.Opts)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on kv.Open")
	} else {
		require.EqualError(t, err, op.ExpErr, "expected different error on kv.Open")
	}
}

type opDBClose struct {
	DB *kv.DB
	ExpErr string
}

func (op opDBClose) Do(t *testing.T, env interface{}) {
	err := op.DB.Close()
	if op.ExpErr == "" {
		require.NoError(t, err, "error closing db")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected close error %q but got: %v", op.ExpErr, err)
	}
}

type opDBSet struct {
	DB *kv.DB
	Key, Value []byte
	ExpErr string
}

func (op opDBSet) Do(t *testing.T, env interface{}) {
	err := op.DB.Set(op.Key, op.Value)
	if op.ExpErr == "" {
		require.NoError(t, err, "error setting value in db")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error setting value in db %q but got: %v", op.ExpErr, err)
	}
}

type opDBGet struct {
	DB *kv.DB
	Key []byte

	ExpValue []byte
	ExpErr string
}

func (op opDBGet) Do(t *testing.T, env interface{}) {
	val, err := op.DB.Get(nil, op.Key)
	if op.ExpErr == "" {
		require.NoError(t, err, "error getting value from db")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error getting value from db %q but got: %v", op.ExpErr, err)
	}

	require.Equal(t, op.ExpValue, val, "read wrong value from db")
}

type opDBPut struct {
	DB *kv.DB
	Key []byte
	Update func(old, new []byte) ([]byte, bool, error)

	ExpOld []byte
	ExpWritten bool
	ExpErr string
}

func (op opDBPut) Do(t *testing.T, env interface{}) {
	old, written, err := op.DB.Put(nil, op.Key, op.Update)
	if op.ExpErr == "" {
		require.NoError(t, err, "error getting value from db")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error getting value from db %q but got: %v", op.ExpErr, err)
	}

	require.Equalf(t, op.ExpWritten, written, "expected written=%v, got %v", op.ExpWritten, written)
	require.Equalf(t, op.ExpOld, old, "expected old=%x, got %x", op.ExpOld, old)
}
