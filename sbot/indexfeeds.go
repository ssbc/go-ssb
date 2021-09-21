// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func newIndexFeedManager(storagePath string) (ssb.IndexFeedManager, error) {
	m := indexFeedManager{indexes: make(map[string]refs.FeedRef), storagePath: storagePath}
	// load previous registered indexes (if any)
	m.load()

	return m, nil
}

type indexFeedManager struct {
	// registered indexes
	indexes     map[string]refs.FeedRef
	storagePath string
}

func constructIndexKey(author refs.FeedRef, msgType string) string {
	return fmt.Sprintf("%s%s", author, msgType)
}

// Method Register keeps track of index feeds so that whenever we publish a new message we also correspondingly publish
// an index message into the registered index feed.
func (manager indexFeedManager) Register(indexFeed, contentFeed refs.FeedRef, msgType string) error {
	// check if the index for this author+type tuple has already been registered
	indexId := constructIndexKey(contentFeed, msgType)
	// an index feed for {contentFeed, msgType} was already registered
	if _, exists := manager.indexes[indexId]; exists {
		return nil
	}

	manager.indexes[indexId] = indexFeed
	err := manager.store()
	if err != nil {
		return err
	}
	return nil
}

// Strategy: persist (as file on disk) which indexes have been created, storing contentFeed+indexFeed+type;
// * contentFeed: the author's pubkey,
// * indexFeed: index feed you publish to, as identified by its SSB1 feedref
// * type: e.g. contact, about
//
// Currently, this is stored all in one file
func (manager indexFeedManager) store() error {
	data, err := json.MarshalIndent(manager.indexes, "", "  ")
	if err != nil {
		return fmt.Errorf("indexFeedManager marshal failed (%w)", err)
	}

	err = os.MkdirAll(manager.storagePath, 0700)
	if err != nil {
		return fmt.Errorf("indexFeedManager mkdir failed (%w)", err)
	}

	indexpath := filepath.Join(manager.storagePath, "indexes.json")
	err = os.WriteFile(indexpath, data, 0700)
	if err != nil {
		return fmt.Errorf("indexFeedManager write indexes file failed (%w)", err)
	}
	return nil
}

func (manager *indexFeedManager) load() error {
	indexpath := filepath.Join(manager.storagePath, "indexes.json")
	// indexes have not yet been persisted to disk
	_, err := os.Stat(indexpath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	data, err := os.ReadFile(indexpath)
	if err != nil {
		return fmt.Errorf("indexFeedManager read indexes file failed (%w)", err)
	}

	err = json.Unmarshal(data, &manager.indexes)
	if err != nil {
		return fmt.Errorf("indexFeedManager unmarshal failed (%w)", err)
	}
	return nil
}

// Method Deregister removes a previously tracked index feed.
// Returns true if feed was found && removed (false if not found)
func (manager indexFeedManager) Deregister(indexFeed refs.FeedRef) (bool, error) {
	var soughtKey string
	for key, feed := range manager.indexes {
		// found the index
		if feed.Equal(indexFeed) {
			soughtKey = key
			break
		}
	}
	if soughtKey != "" {
		delete(manager.indexes, soughtKey)
		err := manager.store()
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// Process looks through all the registered indexes and publishes messages accordingly
func (manager indexFeedManager) Process(m refs.Message) (refs.FeedRef, interface{}, error) {
	// creates index messages after actual messages have been published
	var typed struct {
		Type string
	}
	err := json.Unmarshal(m.ContentBytes(), &typed)
	if err != nil {
		return refs.FeedRef{}, nil, err
	}

	if typed.Type == "" {
		return refs.FeedRef{}, nil, nil
	}

	// lookup correct index
	idxKey := constructIndexKey(m.Author(), typed.Type)
	pubkey, has := manager.indexes[idxKey] // use pubkey to later get publisher
	if !has {
		return refs.FeedRef{}, nil, nil
	}

	content := indexMsg{
		Type: "indexed",
		Indexed: indexed{
			m.Seq(),
			m.Key(),
		},
	}

	return pubkey, content, nil
}

// TODO: make sure this is the right format
type indexed struct {
	Sequence int64           `json:"sequence"`
	Key      refs.MessageRef `json:"key"`
}
type indexMsg struct {
	Type    string  `json:"type"`
	Indexed indexed `json:"indexed"`
}
