package private

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

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

func (mgr *Manager) Pour(ctx context.Context, v interface{}) error {
	msg, ok := v.(refs.Message)
	if !ok {
		return fmt.Errorf("expected type %T, got %T", msg, v)
	}

	var plain json.RawMessage
	err := mgr.Decrypt(ctx, &plain, msg.ContentBytes(), msg.Author())
	if err != nil {
		return nil // couldn't decrypt message; okay!
	}

	_ = plain

	return fmt.Errorf("TODO")
}

func (mgr *Manager) Close(ctx context.Context) error {
	panic("not implemented")
	return nil
}

type EncryptOption func(*encConfig) error

type encConfig struct {
	boxVersion int
	rcpts      []refs.Ref
	// ...
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

var defaultEncConfig encConfig

func init() { // ???
	defaultEncConfig = encConfig{
		boxVersion: 1,
	}
}

func (mgr *Manager) Encrypt(ctx context.Context, content interface{}, opts ...EncryptOption) ([]byte, error) {
	var (
		err       error
		cfg       = defaultEncConfig
		contentBs []byte
		ctxt      []byte
	)

	for _, opt := range opts {
		err := opt(&cfg)
		if err != nil {
			return nil, errors.Wrap(err, "error applying option")
		}
	}

	contentBs, err = json.Marshal(content)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling content")
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

		ctxt, err = bxr.Encrypt(contentBs, rcpts...)
		if err != nil {
			return nil, errors.Wrap(err, "error encrypting message (box1)")
		}
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
		boxmsg, err := bxr.Encrypt(nil, contentBs, mgr.author, nil, ks)
		if err != nil {
			return nil, errors.Wrap(err, "error encrypting message (box1)")
		}

		ctxt = boxmsg
	default:
		panic(fmt.Errorf("invalid box version %q, need 1 or 2", cfg.boxVersion))
	}

	return ctxt, err
}

// bytesSlice attaches the methods of sort.Interface to [][]byte, sorting in increasing order.
type bytesSlice [][]byte

func (p bytesSlice) Len() int { return len(p) }
func (p bytesSlice) Less(i, j int) bool {
	for k := range p[i] {
		if p[i][k] < p[j][k] {
			return true
		}
	}

	return false
}
func (p bytesSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// Sort is a convenience method.
func (p bytesSlice) Sort() { sort.Sort(p) }

func sortAndConcat(bss ...[]byte) []byte {
	bytesSlice(bss).Sort()

	var l int
	for _, bs := range bss {
		l += len(bs)
	}

	var (
		buf = make([]byte, l)
		off int
	)

	for _, bs := range bss {
		off += copy(buf[off:], bs)
	}

	return buf
}

func (mgr *Manager) Decrypt(ctx context.Context, dst interface{}, ctxt []byte, author *refs.FeedRef, opts ...EncryptOption) error {
	var (
		cfg encConfig = defaultEncConfig
		err error
	)

	for _, opt := range opts {
		err = opt(&cfg)
		if err != nil {
			return err
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
			return errors.Wrapf(err, "could not get key for recipient %s", mgr.author.Ref())
		}

		if len(ks) < 1 {
			return fmt.Errorf("no cv25519 secret for feed id %s", mgr.author)
		}
		copy(keyPair.Pair.Secret[:], ks[0].Key)
		copy(keyPair.Pair.Public[:], mgr.author.ID)

		// try decrypt
		bxr := box.NewBoxer(mgr.rand)
		plain, err = bxr.Decrypt(&keyPair, []byte(ctxt))
		if err != nil {
			return errors.Wrap(err, "could not decrypt")
		}

		err = json.Unmarshal(plain, dst)
	case 2: // case box2
		// fetch feed2feed shared key
		keyScheme = keys.SchemeDiffieStyleConvertedED25519
		keyID = sortAndConcat(mgr.author.ID, author.ID)

		ks, err = mgr.keymgr.GetKeys(ctx, keyScheme, keyID)
		if err != nil {
			return errors.Wrapf(err, "could not get key for author %s", author.Ref())
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
		if err != nil {
			return errors.Wrap(err, "could not decrypt")
		}

		err = json.Unmarshal(plain, dst)
	default:
		return fmt.Errorf("expected string to end with either %q or %q", ".box", ".box2")
	}

	return err
}
