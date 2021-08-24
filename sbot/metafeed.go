// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"github.com/ssb-ngi-pointer/go-metafeed"
	"github.com/ssb-ngi-pointer/go-metafeed/metakeys"
	"github.com/ssb-ngi-pointer/go-metafeed/metamngmt"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/slp"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/private/keys"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

// WithMetaFeedMode enables metafeed support.
// It switches the default keypair to bendybutt and initializes the MetaFeed API of the Sbot.
func WithMetaFeedMode(enable bool) Option {
	return func(s *Sbot) error {
		s.enableMetafeeds = enable
		return nil
	}
}

type metaFeedsService struct {
	rxLog margaret.Log
	users multilog.MultiLog
	keys  *keys.Store

	hmacSecret *[32]byte
}

func newMetaFeedService(rxLog margaret.Log, users multilog.MultiLog, keyStore *keys.Store, keypair ssb.KeyPair, hmacSecret *[32]byte) (*metaFeedsService, error) {
	metaKeyPair, ok := keypair.(metakeys.KeyPair)
	if !ok {
		return nil, fmt.Errorf("not a metafeed keypair: %T", keypair)
	}
	if err := checkOrStoreKeypair(keyStore, metaKeyPair); err != nil {
		return nil, fmt.Errorf("failed to initialize keystore with root keypair: %w", err)
	}

	return &metaFeedsService{
		rxLog: rxLog,
		users: users,

		hmacSecret: hmacSecret,

		keys: keyStore,
	}, nil
}

func (s metaFeedsService) CreateSubFeed(mount refs.FeedRef, purpose string, format refs.RefAlgo) (refs.FeedRef, error) {
	// TODO: validate format support (it's on the ebt multiformat branch)

	mountKeyPair, err := loadMetafeedKeyPairFromStore(s.keys, mount)
	if err != nil {
		return refs.FeedRef{}, err
	}

	// create nonce
	var nonce = make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return refs.FeedRef{}, err
	}

	newSubfeedKeyPair, err := metakeys.DeriveFromSeed(mountKeyPair.Seed, string(nonce), format)
	if err != nil {
		return refs.FeedRef{}, err
	}

	newSubfeedAsTFK, err := tfk.Encode(newSubfeedKeyPair.Feed)
	if err != nil {
		return refs.FeedRef{}, err
	}

	// store the singing key
	err = storeKeyPair(s.keys, newSubfeedKeyPair)
	if err != nil {
		return refs.FeedRef{}, err
	}

	// add the subfeed to the list of keys
	listData, err := slp.Encode(newSubfeedAsTFK, []byte(purpose))
	if err != nil {
		return refs.FeedRef{}, err
	}

	subfeedListID := keys.IDFromFeed(mountKeyPair.Feed)
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

	metaPublisher, err := message.OpenPublishLog(s.rxLog, s.users, mountKeyPair, message.SetHMACKey(s.hmacSecret))
	if err != nil {
		return refs.FeedRef{}, err
	}

	addContent := metamngmt.NewAddMessage(mountKeyPair.Feed, newSubfeedKeyPair.Feed, purpose, nonce)

	addMsg, err := metafeed.SubSignContent(newSubfeedKeyPair.PrivateKey, addContent, s.hmacSecret)
	if err != nil {
		return refs.FeedRef{}, err
	}

	_, err = metaPublisher.Publish(addMsg)
	if err != nil {
		return refs.FeedRef{}, err
	}

	return newSubfeedKeyPair.Feed, nil
}

func (s metaFeedsService) TombstoneSubFeed(mount, subfeed refs.FeedRef) error {
	subfeedListing := keys.IDFromFeed(mount)
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
		if f.Metadata.ForFeed.Equal(subfeed) {
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
		return ssb.ErrSubfeedNotActive
	}

	subfeedSigningKey, err := loadMetafeedKeyPairFromStore(s.keys, subfeed)
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

	mountKeyPair, err := loadMetafeedKeyPairFromStore(s.keys, mount)
	if err != nil {
		return err
	}

	metaPublisher, err := message.OpenPublishLog(s.rxLog, s.users, mountKeyPair, message.SetHMACKey(s.hmacSecret))
	if err != nil {
		return err
	}

	tombstoneContent := metamngmt.NewTombstoneMessage(subfeed, mountKeyPair.Feed)
	tombstoneMsg, err := metafeed.SubSignContent(subfeedSigningKey.PrivateKey, tombstoneContent, s.hmacSecret)
	if err != nil {
		return err
	}

	_, err = metaPublisher.Publish(tombstoneMsg)
	if err != nil {
		return err
	}

	return nil
}

func (s metaFeedsService) ListSubFeeds(mount refs.FeedRef) ([]ssb.SubfeedListEntry, error) {
	subfeedListID := keys.IDFromFeed(mount)
	feeds, err := s.keys.GetKeys(keys.SchemeMetafeedSubkey, subfeedListID)
	if err != nil {
		return nil, fmt.Errorf("metafeed list: failed to get listing: %w", err)
	}

	lst := make([]ssb.SubfeedListEntry, len(feeds))
	for i, f := range feeds {

		tfkAndPurpose := slp.Decode(f.Key)

		if n := len(tfkAndPurpose); n != 2 {
			return nil, fmt.Errorf("metafeed/list: invalid key element %d: have %d elements not 2", i, n)
		}

		var feedID tfk.Feed
		err = feedID.UnmarshalBinary(tfkAndPurpose[0])
		if err != nil {
			return nil, err
		}

		feedRef, err := feedID.Feed()
		if err != nil {
			return nil, err
		}

		lst[i] = ssb.SubfeedListEntry{
			Feed:    feedRef,
			Purpose: string(tfkAndPurpose[1]),
		}
	}

	return lst, nil
}

func (s metaFeedsService) Publish(as refs.FeedRef, content interface{}) (refs.Message, error) {
	kp, err := loadMetafeedKeyPairFromStore(s.keys, as)
	if err != nil {
		return nil, fmt.Errorf("publish(as) failed to load signing keypair: %w", err)
	}

	publisher, err := message.OpenPublishLog(s.rxLog, s.users, kp, message.SetHMACKey(s.hmacSecret))
	if err != nil {
		return nil, fmt.Errorf("publish(as) failed to publish message: %w", err)
	}

	return publisher.Publish(content)
}

// utility functions
func loadMetafeedKeyPairFromStore(store *keys.Store, which refs.FeedRef) (metakeys.KeyPair, error) {
	feedAsTFK, err := tfk.Encode(which)
	if err != nil {
		return metakeys.KeyPair{}, err
	}
	dbSubkeyID := keys.ID(feedAsTFK)

	keys, err := store.GetKeys(keys.SchemeFeedMessageSigningKey, dbSubkeyID)
	if err != nil {
		return metakeys.KeyPair{}, err
	}
	if n := len(keys); n != 1 {
		return metakeys.KeyPair{}, fmt.Errorf("expected one signing key but got %d", n)
	}

	keyMaterial := slp.Decode(keys[0].Key)
	if n := len(keyMaterial); n != 2 {
		return metakeys.KeyPair{}, fmt.Errorf("expected two parts of key material but got %d", n)
	}

	return metakeys.KeyPair{
		Feed:       which,
		PrivateKey: ed25519.PrivateKey(keyMaterial[0]),
		Seed:       []byte(keyMaterial[1]),
	}, nil
}

// checkOrStoreKeypair only stores the keypair if it isn't stored yet
func checkOrStoreKeypair(store *keys.Store, kp metakeys.KeyPair) error {
	feedAsTFK, err := tfk.Encode(kp.Feed)
	if err != nil {
		return err
	}
	dbKeypairID := keys.ID(feedAsTFK)

	storedKeyPair, err := store.GetKeys(keys.SchemeFeedMessageSigningKey, dbKeypairID)
	if err == nil && len(storedKeyPair) == 1 {
		// keypair stored, store initalized
		return nil
	}

	// make sure we got error NoSuchKey
	var keystoreErr keys.Error
	if errors.As(err, &keystoreErr) {
		if keystoreErr.Code != keys.ErrorCodeNoSuchKey {
			return fmt.Errorf("checkAndStore: expected NoSuchKey error but got: %w", keystoreErr)
		}
		// ErrorCodeNoSuchKey as intended
	} else {
		return fmt.Errorf("checkAndStore: unexpected keystore failure: %w", err)
	}

	// now store the yet to be set keypair
	return storeKeyPair(store, kp)
}

func storeKeyPair(store *keys.Store, kp metakeys.KeyPair) error {
	feedAsTFK, err := tfk.Encode(kp.Feed)
	if err != nil {
		return err
	}
	dbKeypairID := keys.ID(feedAsTFK)

	keyMaterial, err := slp.Encode(kp.PrivateKey, kp.Seed)
	if err != nil {
		return err
	}

	err = store.AddKey(dbKeypairID, keys.Recipient{
		Key:    keyMaterial,
		Scheme: keys.SchemeFeedMessageSigningKey,
	})
	if err != nil {
		return err
	}

	return nil
}
