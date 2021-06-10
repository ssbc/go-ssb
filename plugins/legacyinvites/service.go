// Package legacyinvites supplies the follow-back sub protocol for new users. Translates to npm:ssb-invite.
package legacyinvites

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger/v3"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	kitlog "go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/invite"
	"go.cryptoscope.co/ssb/repo"
)

// Service holds all the utility functions for invite managment
type Service struct {
	logger kitlog.Logger

	self    refs.FeedRef
	network ssb.Network

	publish    ssb.Publisher
	receiveLog margaret.Log

	mu sync.Mutex
	kv *badger.DB
}

// GuestHandler returns the handler to accept invites
func (s *Service) GuestHandler() muxrpc.Handler {
	return acceptHandler{service: s}
}

// MasterPlugin exposes a muxrpc handler with elevated methods, which can be used to create invites
func (s *Service) MasterPlugin() ssb.Plugin {
	return masterPlug{service: s}
}

// Authorize allows a connection of the guest keypair is known to the service and not yet expired
func (s *Service) Authorize(to refs.FeedRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	kvKey := []byte(storedrefs.Feed(to))

	err := s.kv.Update(func(txn *badger.Txn) error {
		has, err := txn.Get(kvKey)
		if err != nil {
			return fmt.Errorf("invite/auth: failed get guest remote from KV (%w)", err)
		}

		var st inviteState
		err = has.Value(func(val []byte) error {
			return json.Unmarshal(val, &st)
		})
		if err != nil {
			return fmt.Errorf("invite/auth: failed to probe new key (%w)", err)
		}

		if st.Used >= st.Uses {
			txn.Delete(kvKey)
			return fmt.Errorf("invite/auth: invite depleeted")
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

var _ ssb.Authorizer = (*Service)(nil)

// New creates a new invite plugin service
func New(
	logger kitlog.Logger,
	r repo.Interface,
	self refs.FeedRef,
	nw ssb.Network,
	publish ssb.Publisher,
	rlog margaret.Log,
) (*Service, error) {

	kv, err := repo.OpenBadgerDB(r.GetPath("plugin", "legacyinvites"))
	if err != nil {
		return nil, fmt.Errorf("failed to open key-value database: %w", err)
	}

	return &Service{
		logger: logger,

		self:    self,
		network: nw,

		receiveLog: rlog,
		publish:    publish,

		kv: kv,
	}, nil
}

// Close closes the underlying key-value database
func (s Service) Close() error { return s.kv.Close() }

// Create creates a new invite with a note attached and a number of uses before it expires.
func (s Service) Create(uses uint, note string) (*invite.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var inv invite.Token
	err := s.kv.Update(func(txn *badger.Txn) error {

		// roll seed
		var seedRef refs.FeedRef
		for {
			rand.Read(inv.Seed[:])

			inviteKeyPair, err := ssb.NewKeyPair(bytes.NewReader(inv.Seed[:]), refs.RefAlgoFeedSSB1)
			if err != nil {
				return fmt.Errorf("invite/create: generate seeded keypair (%w)", err)
			}

			_, err = txn.Get([]byte(storedrefs.Feed(inviteKeyPair.Id)))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					seedRef = inviteKeyPair.Id
					break
				}
				return fmt.Errorf("invite/create: failed to probe new key (%w)", err)
			}
		}

		// store pub key with params (ties, note)
		st := inviteState{Used: 0}
		st.Uses = uses
		st.Note = note

		data, err := json.Marshal(st)
		if err != nil {
			return fmt.Errorf("invite/create: failed to marshal state data (%w)", err)
		}

		err = txn.Set([]byte(storedrefs.Feed(seedRef)), data)
		if err != nil {
			return fmt.Errorf("invite/create: failed to store state data (%w)", err)
		}

		inv.Peer = s.self
		// TODO: external host configuration?
		inv.Address = s.network.GetListenAddr()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &inv, nil
}

type inviteState struct {
	CreateArguments

	Used uint // how many times this invite was used already
}
