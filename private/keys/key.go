// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package keys

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/hkdf"
)

// Info is generic information to be held by the key store
type Info []byte

// Len returns the length of the info, in bytes when encoded with SLP.
func (info Info) Len() int {
	return 2 + len(info)
}

// Infos is a slice of multiple info items
type Infos []Info

// Len sums up the Len()s of all the infos in the slice
func (is Infos) Len() int {
	var l int

	for _, info := range is {
		l += info.Len()
	}

	return l
}

// TODO: i'm having a feeling this is some SLP pre-cursor code that could be updated by using internal/slp

// Encode writes the slice of infos as one contigous slice of memory
func (is Infos) Encode(out []byte) (int, error) {
	if is.Len() < len(out) {
		return -1, fmt.Errorf("keys: infos/Encode output too small")
	}

	var used int

	for _, info := range is {
		binary.LittleEndian.PutUint16(out[used:], uint16(len(info)))
		used += 2
		used += copy(out[used:], info)
	}

	return used, nil
}

// Metadata stores optional metadata for a key
type Metadata struct {
	GroupRoot *refs.MessageRef
	ForFeed   *refs.FeedRef

	// should be either or but differently typed enums/unions are hard in go :-/
}

// Recipient combines key data with a scheme and some metadata.
type Recipient struct {

	// Key is the actual signature or encryption key.
	Key Key

	// Scheme tells us what it should be used for (shared secret, diffie, etc.)
	Scheme KeyScheme

	Metadata Metadata
}

// Recipients are a list of Recipients
type Recipients []Recipient

// Key holds actual key material
type Key []byte

// Derive returns a new key derived from the internal key data and the passed infos.
func (k Key) Derive(buf []byte, infos Infos, outLen int) (Key, error) {
	// if buffer is too short to hold everything, allocate
	if needed := infos.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	// first the out key because the rest can be freely reused by the caller
	_, out, buf := alloc(buf, outLen)

	// encode info and slice out the written bytes
	l, err := infos.Encode(buf)
	if err != nil {
		return nil, err
	}
	_, infoBs, buf := alloc(buf, l)

	// initialize and perform key derivation
	r := hkdf.New(sha256.New, []byte(k), nil, infoBs)
	if _, err = r.Read(out); err != nil {
		return nil, fmt.Errorf("error deriving key: %w", err)
	}

	// clear intermediate data
	for i := range infoBs {
		infoBs[i] = 0
	}

	return Key(out), err
}

// Keys is a list of keys
type Keys []Key

func alloc(bs []byte, n int) (old, allocd, new []byte) {
	old, allocd, new = bs, bs[:n], bs[n:]
	return
}
