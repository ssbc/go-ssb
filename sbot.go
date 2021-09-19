// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"fmt"

	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

// Publisher embedds a margaret.Log but also exposes a utility that returns the newly created message.
type Publisher interface {
	margaret.Log

	// Publish is a utility wrapper around append which returns the new message
	Publish(content interface{}) (refs.Message, error)
}

// Getter returns a message by its reference
type Getter interface {
	Get(refs.MessageRef) (refs.Message, error)
}

// MultiLogGetter returns a registerd margaret/multilog index by its name
type MultiLogGetter interface {
	GetMultiLog(name string) (multilog.MultiLog, bool)
}

// SimpleIndexGetter returns a registerd margaret/index by its name
type SimpleIndexGetter interface {
	GetSimpleIndex(name string) (librarian.Index, bool)
}

// Indexer combines a bunch of indexing capabilities of a scuttlebot
type Indexer interface {
	MultiLogGetter
	SimpleIndexGetter
	GetIndexNamesSimple() []string
	GetIndexNamesMultiLog() []string
}

// Replicator is used to tell the bot which feeds to copy from other peers and which ones to block
type Replicator interface {
	// Replicate mark a feed for replication and connection acceptance
	Replicate(refs.FeedRef)

	// DontReplicate stops replicating a feed
	DontReplicate(refs.FeedRef)

	Block(refs.FeedRef)
	Unblock(refs.FeedRef)

	Lister() ReplicationLister
}

// ReplicationLister is used by the executing part to get the lists
// TODO: maybe only pass read-only/copies or slices down
type ReplicationLister interface {
	Authorizer
	ReplicationList() *StrFeedSet
	BlockList() *StrFeedSet
}

// Statuser returns status information about the bot, like how many open connections it has (see type Status for more)
type Statuser interface {
	Status() (Status, error)
}

// PeerStatus contains the address of a connected peer and since when it is connected
type PeerStatus struct {
	Addr  string
	Since string
}

// Status contains a bunch of information about a bot
type Status struct {
	PID      int // process id of the bot
	Peers    []PeerStatus
	Blobs    []BlobWant
	Root     int64
	Indicies IndexStates
}

// IndexStates is a slice of index states (for easier sort implementations)
type IndexStates []IndexState

// IndexState informas about the state of a specific index
type IndexState struct {
	Name  string
	State string
}

// ContentNuller if a feed is in a supporting format, it's content can be deleted (or nulled).
type ContentNuller interface {
	NullContent(feed refs.FeedRef, seq uint) error
}

// ErrUnuspportedFormat is returned if a feed format doesn't support nulling content
var ErrUnuspportedFormat = fmt.Errorf("ssb: unsupported format")

// ReplicateUpToResponse is one message of a replicate.upto response.
// also handy to talk about the (latest) state of a single feed
type ReplicateUpToResponse struct {
	ID       refs.FeedRef `json:"id"`
	Sequence int64        `json:"sequence"`
}

// ReplicateUpToResponseSet 's map key is the stringified author
type ReplicateUpToResponseSet map[string]ReplicateUpToResponse

// FeedsWithSeqs returns a source that emits a map with one ReplicateUpToResponse per stored feed in feedIndex.
// TODO: make cancelable and with no RAM overhead when only partially used (iterate on demand)
func FeedsWithSeqs(feedIndex multilog.MultiLog) (ReplicateUpToResponseSet, error) {
	storedFeeds, err := feedIndex.List()
	if err != nil {
		return nil, fmt.Errorf("feedSrc: did not get user list: %w", err)
	}

	allTheFeeds := make([]refs.FeedRef, len(storedFeeds))

	for i, author := range storedFeeds {
		var sr tfk.Feed
		err := sr.UnmarshalBinary([]byte(author))
		if err != nil {
			return nil, fmt.Errorf("feedSrc(%d): invalid storage ref: %w", i, err)
		}

		allTheFeeds[i], err = sr.Feed()
		if err != nil {
			return nil, fmt.Errorf("feedSrc(%d): failed to get feed: %w", i, err)
		}
	}

	return WantedFeedsWithSeqs(feedIndex, allTheFeeds)
}

// WantedFeedsWithSeqs is like FeedsWithSeqs but omits feeds that are not in the wanted list.
func WantedFeedsWithSeqs(feedIndex multilog.MultiLog, wanted []refs.FeedRef) (ReplicateUpToResponseSet, error) {
	var feedsWithSeqs = make(ReplicateUpToResponseSet, len(wanted))

	for i, author := range wanted {

		idxAddr := storedrefs.Feed(author)

		isStored, err := multilog.Has(feedIndex, idxAddr)
		if err != nil {
			return nil, err
		}

		if !isStored {
			feedsWithSeqs[author.String()] = ReplicateUpToResponse{
				ID:       author,
				Sequence: 0,
			}
			continue
		}

		subLog, err := feedIndex.Get(idxAddr)
		if err != nil {
			return nil, fmt.Errorf("feedSrc(%d): did not load sublog: %w", i, err)
		}

		feedsWithSeqs[author.String()] = ReplicateUpToResponse{
			ID:       author,
			Sequence: subLog.Seq() + 1,
		}

	}

	return feedsWithSeqs, nil
}
