// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// Package keys implements an abstract storage system for handling keys of different schemes, like signature and encryption key pairs.
// Its main focus are encryption keys for private-groups and one-to-one direct messages,
// as well as various signature keys for sub/index feeds when using metafeeds.
package keys

import (
	"bytes"
	"context"
	"fmt"

	"go.cryptoscope.co/margaret/indexes"

	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

// Store is the storage layer for signature and encryption key-pairs
type Store struct {
	backend indexes.SetterIndex
}

// NewStore returns a new store using the passed margaret index as its storage backend
func NewStore(b indexes.SetterIndex) *Store {
	return &Store{backend: b}
}

var todoCtx = context.TODO()

// AddKey adds an additional key to the passed ID.
// This is used for example by all the keys for private-groups that can be read which are all stored with the same ID.
func (mgr *Store) AddKey(id ID, r Recipient) error {
	if !r.Scheme.Valid() {
		return Error{Code: ErrorCodeInvalidKeyScheme, Scheme: r.Scheme}
	}

	idxk := &idxKey{
		ks: r.Scheme,
		id: id,
	}

	idxkBytes, err := idxk.MarshalBinary()
	if err != nil {
		return fmt.Errorf("keys/store failed to martial index key: %w", err)
	}

	recps, err := mgr.GetKeys(r.Scheme, id)
	if err != nil {
		if IsNoSuchKey(err) {
			recps = Recipients{}
		} else {
			return fmt.Errorf("error getting old value: %w", err)
		}
	}

	// add new key to existing ones
	recps = append(recps, r)

	return mgr.backend.Set(todoCtx, indexes.Addr(idxkBytes), recps)
}

// SetKey overwrites the recipients for the passed ID and sets a new one
func (mgr *Store) SetKey(id ID, r Recipient) error {
	if !r.Scheme.Valid() {
		return Error{Code: ErrorCodeInvalidKeyScheme, Scheme: r.Scheme}
	}

	idxk := &idxKey{
		ks: r.Scheme,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.backend.Set(todoCtx, indexes.Addr(idxkBs), Recipients{r})
}

// RmKeys deletes all keys for the passed  scheme and ID
func (mgr *Store) RmKeys(ks KeyScheme, id ID) error {
	idxk := &idxKey{
		ks: ks,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.backend.Delete(todoCtx, indexes.Addr(idxkBs))
}

// RmKey removes a single recipient from the passed scheme and id combo
func (mgr *Store) RmKey(ks KeyScheme, id ID, rmKey Recipient) error {
	// load current value
	recps, err := mgr.GetKeys(ks, id)
	if err != nil {
		return fmt.Errorf("error getting current value: %w", err)
	}

	// look for rmKey
	var idx = -1
	for i, r := range recps {
		if bytes.Equal(r.Key, rmKey.Key) {
			idx = i
			break
		}
	}

	if idx < 0 {
		return fmt.Errorf("recpient not in keys list")
	}

	recps = append(recps[:idx], recps[idx+1:]...)

	// update stored entry
	idxk := &idxKey{
		ks: ks,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.backend.Set(todoCtx, indexes.Addr(idxkBs), recps)
}

// GetKeysForMessage loads keys that might be needed to decrypt the passed message
func (mgr *Store) GetKeysForMessage(ks KeyScheme, msg refs.MessageRef) (Recipients, error) {
	idBytes, err := tfk.Encode(msg)
	if err != nil {
		return nil, err
	}
	return mgr.getKeys(ks, ID(idBytes))
}

// GetKeys returns the list of keys held for the passed scheme and ID
func (mgr *Store) GetKeys(ks KeyScheme, id ID) (Recipients, error) {
	return mgr.getKeys(ks, id)
}

func (mgr *Store) getKeys(ks KeyScheme, id ID) (Recipients, error) {
	if !ks.Valid() {
		return nil, Error{Code: ErrorCodeInvalidKeyScheme, Scheme: ks}
	}

	idxk := &idxKey{
		ks: ks,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("key store: failed to marshal index key: %w", err)
	}

	data, err := mgr.backend.Get(todoCtx, indexes.Addr(idxkBs))
	if err != nil {
		return nil, fmt.Errorf("key store: failed to get data from index: %w", err)
	}

	ksIface, err := data.Value()
	if err != nil {
		return nil, fmt.Errorf("key store: failed to unpack index data: %w", err)
	}

	switch tv := ksIface.(type) {
	case Recipients:
		return tv, nil
	case indexes.UnsetValue:
		return nil, Error{
			Code:   ErrorCodeNoSuchKey,
			Scheme: ks,
			ID:     id,
		}
	default:
		return nil, fmt.Errorf("keys store: expected type %T, got %T", Recipients{}, ksIface)
	}
}
