// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ssb-ngi-pointer/go-metafeed/metakeys"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"
)

func TestDefaultKeyPair(t *testing.T) {
	r := require.New(t)

	rpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(rpath)

	repo := New(rpath)

	_, err := DefaultKeyPair(repo, refs.RefAlgoFeedSSB1)
	r.NoError(err, "failed to open key pair")
}

func TestSaveAndLoadBendyButtKeyPair(t *testing.T) {
	r := require.New(t)

	rpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(rpath)

	repo := New(rpath)

	kp, err := DefaultKeyPair(repo, refs.RefAlgoFeedBendyButt)
	r.NoError(err, "failed to open key pair")

	loadedKp, err := DefaultKeyPair(repo, refs.RefAlgoFeedBendyButt)
	r.NoError(err)

	r.True(loadedKp.ID().Equal(kp.ID()))
	r.True(loadedKp.Secret().Equal(kp.Secret()))

	mfkp, ok := loadedKp.(metakeys.KeyPair)
	r.True(ok, "not a metafeed keypair: %T", loadedKp)
	r.Len(mfkp.Seed, metakeys.SeedLength)
}
