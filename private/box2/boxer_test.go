// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

//go:build ignore
// +build ignore

package box2

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ssbc/go-ssb"
	"github.com/ssbc/go-ssb/private/keys"
)

func TestBoxer(t *testing.T) {
	bxr := &Boxer{rand: rand.New(rand.NewSource(161))}

	key := make(keys.Key, KeySize)
	key2 := make(keys.Key, KeySize)

	bxr.rand.Read(key)
	bxr.rand.Read(key2)

	phrase := "squeamish ossifrage"
	author := &ssb.FeedRef{
		ID:   seq(0, 32),
		Algo: "ed25519",
	}

	prev := &ssb.MessageRef{
		Hash: seq(32, 64),
		Algo: "sha256",
	}

	ctxt, err := bxr.Encrypt(nil, []byte(phrase), author, prev, keys.Keys{key, key2})
	require.NoError(t, err, "encrypt")

	plain, err := bxr.Decrypt(nil, ctxt, author, prev, keys.Keys{key})
	require.NoError(t, err, "decrypt")
	require.Equal(t, phrase, string(plain), "wrong words")

	plain, err = bxr.Decrypt(nil, ctxt, author, prev, keys.Keys{key2})
	require.NoError(t, err, "decrypt")
	require.Equal(t, phrase, string(plain), "wrong words")
}
