// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"fmt"

	refs "go.mindeco.de/ssb-refs"
)

// ErrSubfeedNotActive is returned when trying to publish or tombstone an invalid feed
var ErrSubfeedNotActive = fmt.Errorf("ssb: subfeed not marked as active")

// MetaFeeds allows managing and publishing to subfeeds of a metafeed.
type MetaFeeds interface {
	// CreateSubFeed derives a new keypair, stores it in the keystore and publishes a `metafeed/add` message on the metafeed it's mounted on.
	// It takes purpose which will be published and added to the keystore, too.
	// The subfeed will use the pased format.
	CreateSubFeed(mount refs.FeedRef, purpose string, format refs.RefAlgo) (refs.FeedRef, error)

	// TombstoneSubFeed removes the keypair from the store and publishes a `metafeed/tombstone` message to the metafeed it's mounted on.
	// Afterwards the referenced feed is unusable.
	TombstoneSubFeed(mount refs.FeedRef, subfeed refs.FeedRef) error

	// ListSubFeeds returns a list of all _active_ subfeeds of the specified metafeed.
	ListSubFeeds(whose refs.FeedRef) ([]SubfeedListEntry, error)

	// Publish works like normal `Sbot.Publish()` but takes an additional feed reference,
	// which specifies the subfeed on which the content should be published.
	Publish(as refs.FeedRef, content interface{}) (refs.MessageRef, error)
}

// SubfeedListEntry is returned by Metafeeds.ListSubFeeds()
type SubfeedListEntry struct {
	Feed    refs.FeedRef
	Purpose string
}

func (entry SubfeedListEntry) String() string {
	return fmt.Sprintf("%s (%s)", entry.Feed.Ref(), entry.Purpose)
}

// IndexFeedManager allows setting up index feeds
type IndexFeedManager interface {
	// RegisterOnType registers index feed creation on an input feed using the passed msgType to filter messages.
	// input is the feed where messages are read from.
	// output is the index feed where the index messages are published to.
	RegisterOnType(input, output refs.FeedRef, msgType string) error

	Process(refs.Message)

	// TODO List() []refs.FeedRef
	// TODO Stop(refs.FeedRef)
}
