// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func newIndexFeedManager(storagePath string) (ssb.IndexFeedManager, error) {
	m := indexFeedManager{}

	// TODO: load previous registerd indexes

	return m, nil
}

type indexFeedManager struct{}

func (i indexFeedManager) RegisterOnType(input, output refs.FeedRef, msgType string) error {
	return fmt.Errorf("TODO")
}

func (i indexFeedManager) Process(m refs.Message) {
	// TODO: work through all the indexes and do your thing
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

	p.idx.Process(newMsg)

	return seq, nil
}

// Publish is a utility wrapper around append which returns the new message reference key
func (p idxfeedPublisher) Publish(content interface{}) (refs.Message, error) {
	newMsg, err := p.Publisher.Publish(content)
	if err != nil {
		return nil, err
	}

	p.idx.Process(newMsg)

	return newMsg, nil
}
