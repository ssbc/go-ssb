package sbot

import (
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/private/keys"
	refs "go.mindeco.de/ssb-refs"
)

func WithMetaFeedMode(enable bool) Option {
	return func(s *Sbot) error {
		s.enableMetafeeds = enable
		return nil
	}
}

type MetaFeeds interface {
	CreateSubFeed(purpose string, format refs.RefAlgo) (refs.FeedRef, error)
	TombstoneSubFeed(refs.FeedRef) error

	ListSubFeeds() ([]SubfeedListEntry, error)

	Publish(as refs.FeedRef, content interface{}) (refs.MessageRef, error)
}

// stub for disabled mode
type disabledMetaFeeds struct{}

func (disabledMetaFeeds) CreateSubFeed(purpose string, format refs.RefAlgo) (refs.FeedRef, error) {
	return refs.FeedRef{}, fmt.Errorf("metafeeds are disabled")
}

func (disabledMetaFeeds) TombstoneSubFeed(_ refs.FeedRef) error {
	return fmt.Errorf("metafeeds are disabled")
}

func (disabledMetaFeeds) ListSubFeeds() ([]SubfeedListEntry, error) {
	return nil, fmt.Errorf("metafeeds are disabled")
}

func (disabledMetaFeeds) Publish(as refs.FeedRef, content interface{}) (refs.MessageRef, error) {
	return refs.MessageRef{}, fmt.Errorf("metafeeds are disabled")
}

type SubfeedListEntry struct {
	Feed    refs.FeedRef
	Purpose string
}

// actual implemnation

type metaFeedsService struct {
	rxLog margaret.Log
	users multilog.MultiLog
	keys  *keys.Store
}

func newMetaFeedService(rxLog margaret.Log, users multilog.MultiLog, keyStore *keys.Store) (*metaFeedsService, error) {
	return &metaFeedsService{
		rxLog: rxLog,
		users: users,
		keys:  keyStore,
	}, nil
}

func (s metaFeedsService) CreateSubFeed(purpose string, format refs.RefAlgo) (refs.FeedRef, error) {
	// s.keys.GetKeys(keys.SchemeMetafeedSubkey)

	// metakeys.DeriveFromSeed()

	// s.keys.AddKey()
	return refs.FeedRef{}, fmt.Errorf("TODO: create")
}

func (s metaFeedsService) TombstoneSubFeed(_ refs.FeedRef) error {
	return fmt.Errorf("TODO: tombstone")
}

func (s metaFeedsService) ListSubFeeds() ([]SubfeedListEntry, error) {
	// s.keys.GetKeys(keys.SchemeFeedMessageSigningKey)
	return nil, fmt.Errorf("metafeeds are disabled")
}

func (s metaFeedsService) Publish(as refs.FeedRef, content interface{}) (refs.MessageRef, error) {
	return refs.MessageRef{}, fmt.Errorf("TODO: publish as")
}
