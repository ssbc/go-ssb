package gossip

import (
	"net"
	"sync"
	"time"

	refs "github.com/ssbc/go-ssb-refs"
)

const (
	timeoutDoesNotHaveMoreMessages = 2 * time.Minute
)

type ReplicationCompletedFn func(ReplicationResult)

type ReplicationResult string

const (
	ReplicationResultHasMoreMessages         ReplicationResult = "has_more_messages"
	ReplicationResultDoesNotHaveMoreMessages ReplicationResult = "no_more_messages"
)

type FeedTracker struct {
	activeTasks map[string]struct{}
	peerStates  map[string]map[string]replicationResult
	lock        sync.Mutex // locks activeTasks and peerStates
}

func NewFeedTracker() *FeedTracker {
	return &FeedTracker{
		activeTasks: make(map[string]struct{}),
		peerStates:  make(map[string]map[string]replicationResult),
	}
}

func (f *FeedTracker) TryReplicate(peer net.Addr, feed refs.FeedRef) (ReplicationCompletedFn, bool) {
	f.lock.Lock()
	defer f.lock.Unlock()

	fn := f.replicationCompleted(peer, feed)
	shouldReplicate := f.shouldReplicate(peer, feed)

	if shouldReplicate {
		f.activeTasks[f.feedKey(feed)] = struct{}{}
	}

	return fn, shouldReplicate
}

func (f *FeedTracker) shouldReplicate(peer net.Addr, feed refs.FeedRef) bool {
	peerKey := f.peerKey(peer)
	feedKey := f.feedKey(feed)

	feedStates, ok := f.peerStates[peerKey]
	if !ok {
		return true
	}

	state, ok := feedStates[feedKey]
	if !ok {
		return true
	}

	switch state.result {
	case ReplicationResultHasMoreMessages:
		return true
	case ReplicationResultDoesNotHaveMoreMessages:
		return time.Since(state.t) > timeoutDoesNotHaveMoreMessages
	default:
		panic("unknown result")
	}
}

func (f *FeedTracker) replicationCompleted(peer net.Addr, feed refs.FeedRef) ReplicationCompletedFn {
	return func(result ReplicationResult) {
		f.lock.Lock()
		defer f.lock.Unlock()

		peerKey := f.peerKey(peer)
		feedKey := f.feedKey(feed)

		_, ok := f.peerStates[peerKey]
		if !ok {
			f.peerStates[peerKey] = make(map[string]replicationResult)
		}

		f.peerStates[peerKey][feedKey] = replicationResult{
			result: result,
			t:      time.Now(),
		}

		delete(f.activeTasks, f.feedKey(feed))
	}
}

func (f *FeedTracker) peerKey(peer net.Addr) string {
	return peer.String()
}

func (f *FeedTracker) feedKey(feed refs.FeedRef) string {
	return feed.String()
}

type replicationResult struct {
	result ReplicationResult
	t      time.Time
}
