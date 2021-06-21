package sbot

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metakeys"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/slp"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/private/keys"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
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

	rootKeyPair metakeys.KeyPair
}

func newMetaFeedService(rxLog margaret.Log, users multilog.MultiLog, keyStore *keys.Store, keypair ssb.KeyPair) (*metaFeedsService, error) {

	metaKeyPair, ok := keypair.(metakeys.KeyPair)
	if !ok {
		return nil, fmt.Errorf("not a metafeed keypair: %T", keypair)
	}
	return &metaFeedsService{
		rxLog:       rxLog,
		users:       users,
		keys:        keyStore,
		rootKeyPair: metaKeyPair,
	}, nil
}

func (s metaFeedsService) CreateSubFeed(purpose string, format refs.RefAlgo) (refs.FeedRef, error) {
	// TODO: get rootKey?
	// s.keys.GetKeys(keys.SchemeMetafeedSubkey)

	// create nonce
	var nonce = make([]byte, 32)
	_, err := rand.Read(nonce)
	if err != nil {
		return refs.FeedRef{}, err
	}

	newSubfeedKeyPair, err := metakeys.DeriveFromSeed(s.rootKeyPair.Seed, string(nonce), format)
	if err != nil {
		return refs.FeedRef{}, err
	}

	newSubfeedAsTFK, err := tfk.Encode(newSubfeedKeyPair.Feed)
	if err != nil {
		return refs.FeedRef{}, err
	}
	dbSubkeyID := keys.ID(newSubfeedAsTFK)

	// store the singing key
	err = s.keys.AddKey(dbSubkeyID, keys.Recipient{
		Key:    keys.Key(newSubfeedKeyPair.PrivateKey),
		Scheme: keys.SchemeFeedMessageSigningKey,
	})
	if err != nil {
		return refs.FeedRef{}, err
	}

	// add the subfeed to the list of keys
	listData, err := slp.Encode(newSubfeedAsTFK, []byte(purpose))
	if err != nil {
		return refs.FeedRef{}, err
	}

	subfeedListID := keys.IDFromFeed(s.rootKeyPair.Feed)
	err = s.keys.AddKey(subfeedListID, keys.Recipient{
		Key:    keys.Key(listData),
		Scheme: keys.SchemeMetafeedSubkey,

		Metadata: keys.Metadata{
			ForFeed: newSubfeedKeyPair.Feed,
		},
	})
	if err != nil {
		return refs.FeedRef{}, err
	}

	// TODO: hmac setting
	metaPublisher, err := message.OpenPublishLog(s.rxLog, s.users, s.rootKeyPair)
	if err != nil {
		return refs.FeedRef{}, err
	}

	addContent := metamngmt.NewAddMessage(s.rootKeyPair.Feed, newSubfeedKeyPair.Feed, purpose, nonce)

	addMsg, err := metafeed.SubSignContent(newSubfeedKeyPair.PrivateKey, addContent)
	if err != nil {
		return refs.FeedRef{}, err
	}

	addedSubfeedMsg, err := metaPublisher.Publish(addMsg)
	if err != nil {
		return refs.FeedRef{}, err
	}
	fmt.Println("new subfeed published in", addedSubfeedMsg.Ref())

	return newSubfeedKeyPair.Feed, nil
}

func (s metaFeedsService) TombstoneSubFeed(t refs.FeedRef) error {
	subfeedListing := keys.IDFromFeed(s.rootKeyPair.Feed)
	feeds, err := s.keys.GetKeys(keys.SchemeMetafeedSubkey, subfeedListing)
	if err != nil {
		return fmt.Errorf("metafeed list: failed to get subfeed: %w", err)
	}

	var (
		found = false

		// the IDs as which the feed is stored as

		// once in the listing
		subfeedListID keys.Recipient

		// once as the signing key
		subfeedSignKeyID keys.ID
	)

	for i, f := range feeds {
		if f.Metadata.ForFeed.Equal(t) {
			found = true

			tfkAndPurpose := slp.Decode(f.Key)

			if n := len(tfkAndPurpose); n != 2 {
				return fmt.Errorf("metafeed/list: invalid key element %d: have %d elements not 2", i, n)
			}

			subfeedListID = keys.Recipient{
				Key:    f.Key,
				Scheme: keys.SchemeMetafeedSubkey,
			}

			subfeedSignKeyID = tfkAndPurpose[0]

			break
		}
	}

	if !found {
		return fmt.Errorf("subfeed not marked as active")
	}

	subfeedSigningKey, err := s.keys.GetKeys(keys.SchemeFeedMessageSigningKey, subfeedSignKeyID)
	if err != nil {
		return err
	}

	err = s.keys.RmKey(keys.SchemeMetafeedSubkey, subfeedListing, subfeedListID)
	if err != nil {
		return fmt.Errorf("failed to delete subfeed key from listing: %w", err)
	}
	err = s.keys.RmKeys(keys.SchemeFeedMessageSigningKey, subfeedSignKeyID)
	if err != nil {
		return fmt.Errorf("failed to delete subfeed signing key: %w", err)
	}

	metaPublisher, err := message.OpenPublishLog(s.rxLog, s.users, s.rootKeyPair)
	if err != nil {
		return err
	}

	tombstoneContent := metamngmt.NewTombstoneMessage(t)
	tombstoneMsg, err := metafeed.SubSignContent(ed25519.PrivateKey(subfeedSigningKey[0].Key), tombstoneContent)
	if err != nil {
		return err
	}

	tombstonedSubfeedMsg, err := metaPublisher.Publish(tombstoneMsg)
	if err != nil {
		return err
	}
	fmt.Println("subfeed tombstone published in", tombstonedSubfeedMsg.Ref())

	return nil
}

func (s metaFeedsService) ListSubFeeds() ([]SubfeedListEntry, error) {
	subfeedListID := keys.IDFromFeed(s.rootKeyPair.Feed)
	feeds, err := s.keys.GetKeys(keys.SchemeMetafeedSubkey, subfeedListID)
	if err != nil {
		return nil, fmt.Errorf("metafeed list: failed to get listing: %w", err)
	}

	lst := make([]SubfeedListEntry, len(feeds))
	for i, f := range feeds {

		tfkAndPurpose := slp.Decode(f.Key)

		if n := len(tfkAndPurpose); n != 2 {
			return nil, fmt.Errorf("metafeed/list: invalid key element %d: have %d elements not 2", i, n)
		}

		var feedID tfk.Feed
		err = feedID.UnmarshalBinary(tfkAndPurpose[0])

		feedRef, err := feedID.Feed()
		if err != nil {
			return nil, err
		}

		lst[i] = SubfeedListEntry{
			Feed:    feedRef,
			Purpose: string(tfkAndPurpose[1]),
		}
	}

	return lst, nil
}

func (s metaFeedsService) Publish(as refs.FeedRef, content interface{}) (refs.MessageRef, error) {
	feedAsTFK, err := tfk.Encode(as)
	if err != nil {
		return refs.MessageRef{}, err
	}
	dbSubkeyID := keys.ID(feedAsTFK)

	keys, err := s.keys.GetKeys(keys.SchemeFeedMessageSigningKey, dbSubkeyID)
	if err != nil {
		return refs.MessageRef{}, err
	}
	if n := len(keys); n != 1 {
		return refs.MessageRef{}, fmt.Errorf("expected one signing key but got %d", n)
	}

	kp := metakeys.KeyPair{
		Feed:       as,
		PrivateKey: ed25519.PrivateKey(keys[0].Key),
	}

	// TODO: hmac setting
	publisher, err := message.OpenPublishLog(s.rxLog, s.users, kp)
	if err != nil {
		return refs.MessageRef{}, err
	}

	return publisher.Publish(content)
}
