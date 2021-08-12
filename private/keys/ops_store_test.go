// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package keys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type opStoreSetKey struct {
	Mgr    *Store
	Scheme KeyScheme
	ID     ID
	Key    Key

	ExpErr string
}

func (op opStoreSetKey) Do(t *testing.T, env interface{}) {
	r := Recipient{
		Key:    op.Key,
		Scheme: op.Scheme,
	}
	err := op.Mgr.SetKey(op.ID, r)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on mgr.SetKey")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q on setkey, get %v", op.ExpErr, err)
	}
}

type opStoreAddKey struct {
	Mgr    *Store
	Scheme KeyScheme
	ID     ID
	Key    Key

	ExpErr string
}

func (op opStoreAddKey) Do(t *testing.T, env interface{}) {

	r := Recipient{
		Key:    op.Key,
		Scheme: op.Scheme,
	}
	err := op.Mgr.AddKey(op.ID, r)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on mgr.AddKey")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q on addkey, get %v", op.ExpErr, err)
	}
}

type opStoreRmKeys struct {
	Mgr    *Store
	Scheme KeyScheme
	ID     ID

	ExpErr string
}

func (op opStoreRmKeys) Do(t *testing.T, env interface{}) {
	err := op.Mgr.RmKeys(op.Scheme, op.ID)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error removing a key")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q on RmKey, got: %v", op.ExpErr, err)
	}
}

type opStoreGetKeys struct {
	Mgr    *Store
	Scheme KeyScheme
	ID     ID

	ExpRecps Recipients
	ExpErr   string
}

func (op opStoreGetKeys) Do(t *testing.T, _ interface{}) {
	recps, err := op.Mgr.GetKeys(op.Scheme, op.ID)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error querying keys")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q querying keys, but got: %v", op.ExpErr, err)
	}
	require.Len(t, recps, len(op.ExpRecps), "number of keys missmatch")
	require.Equal(t, op.ExpRecps, recps, "keys mismatch")
}
