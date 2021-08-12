// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package keys

import (
	"context"
	"testing"

	"github.com/dgraph-io/badger/v3"
	"github.com/stretchr/testify/require"
	librarian "go.cryptoscope.co/margaret/indexes"
	libbadger "go.cryptoscope.co/margaret/indexes/badger"
	"go.cryptoscope.co/ssb/repo"
	"modernc.org/kv"
)

type opIndexNew struct {
	DB   **badger.DB
	Type interface{}

	Index *librarian.SeqSetterIndex
}

func (op opIndexNew) Do(t *testing.T, env interface{}) {
	*op.Index = libbadger.NewIndex(*op.DB, op.Type)
	require.NotNil(t, *op.Index, "libbadger.NewIndex returned nil")
}

type opIndexGet struct {
	Index *librarian.SeqSetterIndex
	Addr  librarian.Addr

	ExpValue  interface{}
	ExpGetErr string
	ExpObsErr string
}

func (op opIndexGet) Do(t *testing.T, env interface{}) {
	obs, gerr := (*op.Index).Get(context.TODO(), op.Addr)
	if op.ExpGetErr == "" {
		require.NoError(t, gerr, "unexpected error on idx.Get")
	} else {
		require.EqualError(t, gerr, op.ExpGetErr, "expected different error on idx.Get")
	}

	v, verr := obs.Value()
	if op.ExpObsErr == "" {
		require.NoError(t, verr, "unexpected error opening observable after idx.Get")
	} else {
		require.EqualError(t, verr, op.ExpObsErr, "expected different error opening observable after idx.Get")
	}

	require.Equal(t, op.ExpValue, v, "wrong value for addr:%q", op.Addr)
}

type opDBCreate struct {
	Name string

	ExpErr string
	DB     **badger.DB
}

func (op opDBCreate) Do(t *testing.T, env interface{}) {
	var err error

	*(op.DB), err = repo.OpenBadgerDB(op.Name)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on kv.Create")
	} else {
		require.EqualError(t, err, op.ExpErr, "expected different error on kv.Create")
	}
}

type opDBOpen struct {
	Name string

	ExpErr string
	DB     **badger.DB
}

func (op opDBOpen) Do(t *testing.T, env interface{}) {
	var err error
	*(op.DB), err = repo.OpenBadgerDB(op.Name)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on kv.Open")
	} else {
		require.EqualError(t, err, op.ExpErr, "expected different error on kv.Open")
	}
}

type opDBClose struct {
	DB     *kv.DB
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
	DB         *kv.DB
	Key, Value []byte
	ExpErr     string
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
	DB  **badger.DB
	Key []byte

	Log bool

	ExpValue []byte
	ExpErr   string
}

func (op opDBGet) Do(t *testing.T, env interface{}) {
	(*op.DB).View(func(txn *badger.Txn) error {
		val, err := txn.Get(op.Key)
		if op.ExpErr == "" {
			require.NoError(t, err, "error getting value from db")
		} else {
			require.EqualErrorf(t, err, op.ExpErr, "expected error getting value from db %q but got: %v", op.ExpErr, err)
			return nil
		}
		data, err := val.ValueCopy(nil)
		if err != nil {
			require.NoError(t, err, "did not get value")
			return nil
		}
		if op.Log {
			t.Logf("DB.Get - Key:%x Value:%x Exp:%x", op.Key, data, op.ExpValue)
		}
		require.Equal(t, op.ExpValue, data, "read wrong value from db")
		return nil
	})

}
