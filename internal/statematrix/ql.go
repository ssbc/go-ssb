// SPDX-License-Identifier: MIT

/*Package statematrix stores and provides useful operations on an state matrix for a epidemic broadcast tree protocol.

The state matrix represents multiple _network frontiers_ (or vector clock).

This version uses a SQL because that seems much handier to handle such an irregular sparse matrix.

Q:
* do we need a 2nd _told us about_ table?

*/
package statematrix

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

type StateMatrix struct {
	basePath string

	self string // whoami

	mu   sync.Mutex
	open currentFrontiers
}

// map[peer refernce]frontier
type currentFrontiers map[string]ssb.NetworkFrontier

func New(base string, self *refs.FeedRef) (*StateMatrix, error) {

	os.MkdirAll(base, 0700)

	sm := StateMatrix{
		basePath: base,

		self: self.Ref(),

		open: make(currentFrontiers),
	}

	_, err := sm.loadFrontier(self)
	if err != nil {
		return nil, err
	}

	return &sm, nil
}

/*
func (sm *StateMatrix) Open(peer *refs.FeedRef) (ssb.NetworkFrontier, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.loadFrontier(peer)
}
*/

func (sm *StateMatrix) stateFileName(peer *refs.FeedRef) (string, error) {
	peerTfk, err := tfk.Encode(peer)
	if err != nil {
		return "", err
	}

	peerFileName := filepath.Join(sm.basePath, fmt.Sprintf("%x", peerTfk))
	return peerFileName, nil
}

func (sm *StateMatrix) loadFrontier(peer *refs.FeedRef) (ssb.NetworkFrontier, error) {
	curr, has := sm.open[peer.Ref()]
	if has {
		curr = make(ssb.NetworkFrontier)
		return curr, nil
	}

	peerFileName, err := sm.stateFileName(peer)
	if err != nil {
		return nil, err
	}

	peerFile, err := os.Open(peerFileName)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		curr = make(ssb.NetworkFrontier)
	} else {
		defer peerFile.Close()

		var nf ssb.NetworkFrontier
		err = json.NewDecoder(peerFile).Decode(&nf)
		if err != nil {
			return nil, err
		}
	}

	sm.open[peer.Ref()] = curr
	return curr, nil
}

func (sm *StateMatrix) SaveAndClose(peer *refs.FeedRef) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.saveAndClose(peer.Ref())
}

func (sm *StateMatrix) saveAndClose(peer string) error {
	parsed, err := refs.ParseFeedRef(peer)
	if err != nil {
		return err
	}

	nf, has := sm.open[peer]
	if !has {
		return nil
	}

	peerFileName, err := sm.stateFileName(parsed)
	if err != nil {
		return err
	}

	// truncate the file for overwriting, create it if it doesnt exist
	peerFile, err := os.OpenFile(peerFileName, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		return err
	}

	err = json.NewEncoder(peerFile).Encode(nf)
	if err != nil {
		return err
	}

	delete(sm.open, peer)
	return peerFile.Close()
}

type HasLongerResult struct {
	Peer *refs.FeedRef
	Feed *refs.FeedRef
	Len  uint64
}

func (hlr HasLongerResult) String() string {
	return fmt.Sprintf("%s: %s:%d", hlr.Peer.ShortRef(), hlr.Feed.ShortRef(), hlr.Len)
}

func (sm *StateMatrix) HasLonger() ([]HasLongerResult, error) {
	var err error

	sm.mu.Lock()
	defer sm.mu.Unlock()

	selfNf, has := sm.open[sm.self]
	if !has {
		return nil, nil
	}

	var res []HasLongerResult

	for peer, theirNf := range sm.open {

		for feed, note := range selfNf {

			theirNote, has := theirNf[feed]
			if !has {
				continue
			}

			if theirNote.Seq > note.Seq {
				var hlr HasLongerResult
				hlr.Len = uint64(theirNote.Seq)

				hlr.Peer, err = refs.ParseFeedRef(peer)
				if err != nil {
					return nil, err
				}

				hlr.Feed, err = refs.ParseFeedRef(feed)
				if err != nil {
					return nil, err
				}

				res = append(res, hlr)
			}

		}
	}

	return res, nil
}

// all the feeds a peer wants to recevie messages for
func (sm *StateMatrix) WantsList(peer *refs.FeedRef) ([]*refs.FeedRef, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	nf, err := sm.loadFrontier(peer)
	if err != nil {
		return nil, err
	}

	var res []*refs.FeedRef

	for feedStr, note := range nf {
		if note.Receive {
			feed, err := refs.ParseFeedRef(feedStr)
			if err != nil {
				return nil, fmt.Errorf("wantList: failed to parse feed entry %q: %w", feedStr, err)
			}
			res = append(res, feed)
		}
	}

	return res, nil
}

func (sm *StateMatrix) WantsFeed(peer, feed *refs.FeedRef, weHave uint64) (bool, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	nf, err := sm.loadFrontier(peer)
	if err != nil {
		return false, err
	}

	n, has := nf[feed.Ref()]
	if !has {
		return false, nil
	}

	return n.Receive, nil
}

// Changed returns which feeds have newer messages since last update
func (sm *StateMatrix) Changed(self, peer *refs.FeedRef) (ssb.NetworkFrontier, error) {

	sm.mu.Lock()
	defer sm.mu.Unlock()

	changedFrontier := make(ssb.NetworkFrontier)

	selfNf, has := sm.open[sm.self]
	if !has {
		return changedFrontier, nil
	}

	peerNf, has := sm.open[sm.self]
	if !has {
		return changedFrontier, nil
	}

	for theirFeed, theirNote := range peerNf {

		for myFeed, myNote := range selfNf {
			if theirFeed != myFeed {
				continue
			}

			if !theirNote.Receive {
				continue
			}

			if myNote.Seq > theirNote.Seq {
				changedFrontier[myFeed] = myNote
			}
		}

	}

	return changedFrontier, nil
}

type ObservedFeed struct {
	Feed *refs.FeedRef

	ssb.Note
}

func (sm *StateMatrix) Fill(who *refs.FeedRef, feeds []ObservedFeed) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	nf, err := sm.loadFrontier(who)
	if err != nil {
		return err
	}

	for _, of := range feeds {

		if of.Replicate {
			nf[of.Feed.Ref()] = of.Note
		} else {
			// seq == -1 means drop it
			delete(nf, of.Feed.Ref())
		}
	}

	sm.open[who.Ref()] = nf
	return nil
}

func (sm *StateMatrix) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for peer := range sm.open {
		sm.saveAndClose(peer)
	}

	return nil
}
