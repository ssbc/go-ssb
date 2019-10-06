package keys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type opManagerSetKey struct {
	Mgr *Manager
	Type Type
	ID ID
	Key Key

	ExpErr string
}

func (op opManagerSetKey) Do(t *testing.T, env interface{}) {
	err := op.Mgr.SetKey(op.Type, op.ID, op.Key)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on mgr.SetKey")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q on setkey, get %v", op.ExpErr, err)
	}
}

type opManagerAddKey struct {
	Mgr *Manager
	Type Type
	ID ID
	Key Key

	ExpErr string
}

func (op opManagerAddKey) Do(t *testing.T, env interface{}) {
	err := op.Mgr.AddKey(op.Type, op.ID, op.Key)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error on mgr.AddKey")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q on addkey, get %v", op.ExpErr, err)
	}
}

type opManagerRmKeys struct {
	Mgr *Manager
	Type Type
	ID ID

	ExpErr string
}

func (op opManagerRmKeys) Do(t *testing.T, env interface{}) {
	err := op.Mgr.RmKeys(op.Type, op.ID)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error removing a key")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q on RmKey, got: %v", op.ExpErr, err)
	}
}

type opManagerGetKeys struct {
	Mgr *Manager
	Type Type
	ID ID

	ExpKeys Keys
	ExpErr string
}

func (op opManagerGetKeys) Do(t *testing.T, env interface{}) {
	keys, err := op.Mgr.GetKeys(op.Type, op.ID)
	if op.ExpErr == "" {
		require.NoError(t, err, "unexpected error querying keys")
	} else {
		require.EqualErrorf(t, err, op.ExpErr, "expected error %q querying keys, but got: %v", op.ExpErr, err)
	}

	require.Equal(t, op.ExpKeys, *keys, "keys mismatch")
}
