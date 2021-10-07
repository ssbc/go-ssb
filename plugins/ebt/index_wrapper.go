// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ebt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

// findAuthorForIndexfeed finds the author an index feed _indexes over_ (so on which feed the messages actually happen).
// TODO: this is a bit brute force and the map is unbound, too... to be replaced with smarter persisted indexed lookups
func (r *Replicate) findAuthorForIndexfeed(ctx context.Context, indexFeed refs.FeedRef) (refs.FeedRef, error) {
	// check the cache first
	r.idx2authCacheMu.Lock()
	if author, has := r.idx2authCache[indexFeed]; has {
		r.idx2authCacheMu.Unlock()
		return author, nil
	}
	r.idx2authCacheMu.Unlock()

	// no hit.. so first, find the metafeed that houses the index feed
	// a message on that should give us the metadata we are looking for
	idxBucket, err := r.graph.Metafeed(indexFeed)
	if err != nil {
		return refs.FeedRef{}, err
	}

	// construct the virtual log to read the messages on the bucket of index feeds
	idxIndex, err := r.userFeeds.Get(storedrefs.Feed(idxBucket))
	if err != nil {
		return refs.FeedRef{}, err
	}
	idxMessages := mutil.Indirect(r.receiveLog, idxIndex)

	idxMsgsQry, err := idxMessages.Query()
	if err != nil {
		return refs.FeedRef{}, err
	}

	// iterate over the result
	for {
		v, err := idxMsgsQry.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			return refs.FeedRef{}, err
		}

		msg, ok := v.(refs.Message)
		if !ok {
			return refs.FeedRef{}, fmt.Errorf("unexpected message type: %T", v)
		}

		// decode the add message
		var addMsg metamngmt.AddDerived
		err = metafeed.VerifySubSignedContent(msg.ContentBytes(), &addMsg)
		if err != nil {
			continue
		}

		// we found the metafeed add message for the index feed we are looking for
		if addMsg.SubFeed.Equal(indexFeed) {
			qryStr, ok := addMsg.GetMetadata("query")
			if !ok {
				return refs.FeedRef{}, fmt.Errorf("found add message for %s but has no metadata", indexFeed)
			}

			// decode the metadata field for the ql-0 query data
			var qry ssb.MetadataQuery
			err = json.Unmarshal([]byte(qryStr), &qry)
			if err != nil {
				return refs.FeedRef{}, err
			}

			// mark the result in the cache to not do this again
			r.idx2authCacheMu.Lock()
			r.idx2authCache[indexFeed] = qry.Author
			r.idx2authCacheMu.Unlock()
			return qry.Author, nil
		}
	}

	return refs.FeedRef{}, fmt.Errorf("failed to find author for indexfeed: %s", indexFeed)
}

// wraps a sink and changes the write call such that it it merges the indexed with the refrenced message into an array ([indexed, actual])
func (r *Replicate) indexedWrapper(snk muxrpc.ByteSinker, author refs.FeedRef) (muxrpc.ByteSinker, error) {
	authorIndex, err := r.userFeeds.Get(storedrefs.Feed(author))
	if err != nil {
		return nil, err
	}

	return &indexedSender{
		ByteSinker: snk,
		actualLog:  mutil.Indirect(r.receiveLog, authorIndex),
	}, nil
}

type indexedSender struct {
	// the sinker that sends to the network
	muxrpc.ByteSinker

	// the full messages for this author
	actualLog margaret.Log
}

func (sink *indexedSender) Write(p []byte) (int, error) {
	var indexedMsg struct {
		Author  refs.FeedRef
		Content ssb.IndexedMessage
	}
	err := json.Unmarshal(p, &indexedMsg)
	if err != nil {
		return -1, err
	}

	// margaret indexes are 0-based
	seq := indexedMsg.Content.Indexed.Sequence - 1
	actualMsgV, err := sink.actualLog.Get(seq)
	if err != nil {
		return -1, fmt.Errorf("index sender failed to get referenced message (%s:%d): %w", indexedMsg.Author.String(), seq, err)
	}

	actualMsg := actualMsgV.(refs.Message)
	if !actualMsg.Key().Equal(indexedMsg.Content.Indexed.Key) {
		return -1, fmt.Errorf("indexed message does not reference author message as expected")
	}

	// now stitch indexed and acutal message together
	// as [indexed, actual], ....
	stitchedMsg := []byte{'['}
	stitchedMsg = append(stitchedMsg, p...)
	stitchedMsg = append(stitchedMsg, ',')
	stitchedMsg = append(stitchedMsg, actualMsg.ValueContentJSON()...)
	stitchedMsg = append(stitchedMsg, ']')

	return sink.ByteSinker.Write(stitchedMsg)
}
