package private

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/ed25519"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
	"go.cryptoscope.co/ssb/private/box"
	"go.cryptoscope.co/ssb/private/box2"
)

type Manager struct {
	log    margaret.Log // Q: ???? which log is this?!
	author *refs.FeedRef

	keymgr *keys.Store
	rand   io.Reader
}

type EncryptOption func(*encConfig) error

type encConfig struct {
	boxVersion int
	rcpts      []refs.Ref
}

var defaultEncConfig = encConfig{
	boxVersion: 1,
}

// TODO: If one of the refs is not a feedref, set box2
func WithRecipients(rcpts ...refs.Ref) EncryptOption {
	return func(cfg *encConfig) error {
		cfg.rcpts = append(cfg.rcpts, rcpts...)
		return nil
	}
}

func WithBox2() EncryptOption {
	return func(cfg *encConfig) error {
		cfg.boxVersion = 2
		return nil
	}
}

func (mgr *Manager) Encrypt(ctx context.Context, content []byte, opts ...EncryptOption) ([]byte, error) {
	var cfg = defaultEncConfig

	for _, opt := range opts {
		err := opt(&cfg)
		if err != nil {
			return nil, errors.Wrap(err, "error applying option")
		}
	}

	switch cfg.boxVersion {
	case 1:
		var (
			bxr   = box.NewBoxer(mgr.rand)
			rcpts = make([]*refs.FeedRef, len(cfg.rcpts))
			ok    bool
		)

		for i := range cfg.rcpts {
			rcpts[i], ok = cfg.rcpts[i].(*refs.FeedRef)
			if !ok {
				return nil, fmt.Errorf("box1 can only send to feed ids")
			}
		}

		ctxt, err := bxr.Encrypt(content, rcpts...)
		return ctxt, errors.Wrap(err, "error encrypting message (box1)")
	case 2:
		// first, look up keys
		var (
			ks        = make(keys.Recipients, 0, len(cfg.rcpts))
			keyScheme keys.KeyScheme
			keyID     keys.ID
		)

		for _, rcpt := range cfg.rcpts {
			switch ref := rcpt.(type) {
			case *refs.FeedRef:
				keyScheme = keys.SchemeDiffieStyleConvertedED25519
				keyID = keys.ID(sortAndConcat(mgr.author.ID, ref.ID))
			case *refs.MessageRef:
				// TODO: maybe verify this is a group message?
				keyScheme = keys.SchemeLargeSymmetricGroup
				keyID = keys.ID(sortAndConcat(ref.Hash)) // actually just copy
			}

			ks_, err := mgr.keymgr.GetKeys(ctx, keyScheme, keyID)
			if err != nil {
				return nil, errors.Wrapf(err, "could not get key for recipient %s", rcpt.Ref())
			}

			ks = append(ks, ks_...)
		}

		// then, encrypt message
		bxr := box2.NewBoxer(mgr.rand)
		// TODO: previous?!
		ctxt, err := bxr.Encrypt(nil, content, mgr.author, nil, ks)
		return ctxt, errors.Wrap(err, "error encrypting message (box1)")
	default:
		return nil, fmt.Errorf("invalid box version %q, need 1 or 2", cfg.boxVersion)
	}
}

func (mgr *Manager) Decrypt(ctx context.Context, ctxt []byte, author *refs.FeedRef, opts ...EncryptOption) ([]byte, error) {
	var (
		cfg encConfig = defaultEncConfig
		err error
	)

	for _, opt := range opts {
		err = opt(&cfg)
		if err != nil {
			return nil, err
		}
	}

	// determine candidate keys
	var (
		ks        keys.Recipients
		keyScheme keys.KeyScheme
		keyID     keys.ID
		plain     []byte
	)

	switch cfg.boxVersion {
	case 1: // case box1 ?????
		keyPair := ssb.KeyPair{Id: mgr.author}
		keyPair.Pair.Secret = make(ed25519.PrivateKey, ed25519.PrivateKeySize)
		keyPair.Pair.Public = make(ed25519.PublicKey, ed25519.PublicKeySize)

		// read secret DH key from database
		keyScheme = keys.SchemeDiffieStyleConvertedED25519
		keyID = sortAndConcat(mgr.author.ID)

		ks, err = mgr.keymgr.GetKeys(ctx, keyScheme, keyID)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get key for recipient %s", mgr.author.Ref())
		}

		if len(ks) < 1 {
			return nil, fmt.Errorf("no cv25519 secret for feed id %s", mgr.author)
		}
		copy(keyPair.Pair.Secret[:], ks[0].Key)
		copy(keyPair.Pair.Public[:], mgr.author.ID)

		// try decrypt
		bxr := box.NewBoxer(mgr.rand)
		plain, err = bxr.Decrypt(&keyPair, []byte(ctxt))
		return plain, errors.Wrap(err, "could not decrypt")
	case 2: // case box2
		// fetch feed2feed shared key
		keyScheme = keys.SchemeDiffieStyleConvertedED25519
		keyID = sortAndConcat(mgr.author.ID, author.ID)

		ks, err = mgr.keymgr.GetKeys(ctx, keyScheme, keyID)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get key for author %s", author.Ref())
		}

		// fetch groups author is member of
		// TODO: we don't track this yet

		// fetch group keys
		// TODO: we don't know which group keys to fetch yet

		// try decrypt
		// TODO pass in buffer
		// TODO set infos
		bxr := box2.NewBoxer(mgr.rand)
		plain, err = bxr.Decrypt(nil, []byte(ctxt), author, nil, ks)
		return plain, errors.Wrap(err, "could not decrypt")
	default:
		return nil, fmt.Errorf("expected string to end with either %q or %q", ".box", ".box2")
	}

}
