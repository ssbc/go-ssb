// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func newIndexFeedManager(storagePath string, mf ssb.MetaFeeds) (ssb.IndexFeedManager, error) {
	m := indexFeedManager{}

	// TODO: load previous registerd indexes (see metafeed publishAs to see how to create a publisher)
	// TODO: add tests for that (register, stop bot, start bot, check is registerd)

	return m, nil
}

type indexFeedManager struct {
	// registerd indexes
	// map key could be 'input:msgtype'
	indexes map[string]ssb.Publisher
}

func (i indexFeedManager) RegisterOnType(input refs.FeedRef, msgType string) error {
	// TODO: check is already registerd

	// if not, create new subfeed with metadata
	// persist configuration (input feed, msgType and output feed)

	return fmt.Errorf("TODO: RegisterOnType")
}

// Process looks through all the registerd indexes and publishes messages accordingly
func (i indexFeedManager) Process(m refs.Message) error {

	var typed struct {
		Type string
	}
	err := json.Unmarshal(m.ContentBytes(), &typed)
	if err != nil {
		return nil
	}

	if typed.Type == "" {
		return nil
	}

	idxKey := m.Author().String() + typed.Type
	publish, has := i.indexes[idxKey]
	if !has {
		return nil
	}

	// TODO: make sure this is the right format
	type indexMsg struct {
		Type    string `json:"type"`
		Indexed struct {
			Sequence int `json:"sequence"`
			Key      int `json:"key"`
		}
	}
	content := indexMsg{
		Type: "indexed",

		// Indexed: {
		// 	Sequence: msg.Seq(),
		// 	Hash:     msg.Key(),
		// },
	}
	_, err = publish.Publish(content)
	if err != nil {
		return fmt.Errorf("failed to publish index message")
	}
	return nil
}

type idxfeedPublisher struct {
	ssb.Publisher

	receiveLog margaret.Log

	idx ssb.IndexFeedManager
}

func newWrappedPublisher(p ssb.Publisher, rxlog margaret.Log, i ssb.IndexFeedManager) ssb.Publisher {
	return idxfeedPublisher{
		Publisher: p,

		idx:        i,
		receiveLog: rxlog,
	}
}

// Append appends a new entry to the log
func (p idxfeedPublisher) Append(content interface{}) (int64, error) {
	seq, err := p.Publisher.Append(content)
	if err != nil {
		return margaret.SeqErrored, err
	}

	val, err := p.receiveLog.Get(seq)
	if err != nil {
		return margaret.SeqErrored, fmt.Errorf("publish: failed to get new stored message: %w", err)
	}

	newMsg, ok := val.(refs.Message)
	if !ok {
		return margaret.SeqErrored, fmt.Errorf("publish: unsupported keyer %T", val)
	}

	if err := p.idx.Process(newMsg); err != nil {
		return margaret.SeqErrored, err
	}

	return seq, nil
}

// Publish is a utility wrapper around append which returns the new message reference key
func (p idxfeedPublisher) Publish(content interface{}) (refs.Message, error) {
	newMsg, err := p.Publisher.Publish(content)
	if err != nil {
		return nil, err
	}

	if err := p.idx.Process(newMsg); err != nil {
		return nil, err
	}

	return newMsg, nil
}
