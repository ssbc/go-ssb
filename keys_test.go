// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "github.com/ssbc/go-ssb-refs"
)

func TestSaveKeyPair(t *testing.T) {
	fname := path.Join(os.TempDir(), "secret")

	keys, err := NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	require.NoError(t, err)
	err = SaveKeyPair(keys, fname)
	require.NoError(t, err)
	defer os.Remove(fname)

	stat, err := os.Stat(fname)
	require.NoError(t, err)
	assert.Equal(t, SecretPerms, stat.Mode(), "file permissions")
}

func TestLoadKeyPair(t *testing.T) {
	tests := []struct {
		Name   string
		Perms  os.FileMode
		HasIncorrectPermissions bool
	}{
		{
			"Success",
			SecretPerms,
			false,
		},
		{
			"Bad file permissions, should be corrected",
			0777,
			true,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			fname := path.Join(os.TempDir(), "secret")

			keys, err := NewKeyPair(nil, refs.RefAlgoFeedSSB1)
			require.NoError(t, err)
			err = SaveKeyPair(keys, fname)
			require.NoError(t, err)
			defer os.Remove(fname)

			err = os.Chmod(fname, test.Perms)
			require.NoError(t, err)

			_, err = LoadKeyPair(fname)
			if test.HasIncorrectPermissions {
				info, err := os.Stat(fname)
				assert.NoError(t, err)
				assert.EqualValues(t, info.Mode().Perm(), SecretPerms, "incorrect permissions have not been corrected automatically")
				return
			}
			assert.NoError(t, err)
		})
	}
}
