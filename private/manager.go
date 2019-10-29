package private

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
	"go.cryptoscope.co/ssb/message/msgkit"
	"go.cryptoscope.co/ssb/private/box"
	"go.cryptoscope.co/ssb/private/box2"
)

type Manager struct {
	log    margaret.Log
	author *ssb.FeedRef

	keymgr *keys.Manager
}

func (mgr *Manager) Pour(ctx context.Context, v interface{}) error {
	msg, ok := v.(ssb.Message)
	if !ok {
		return fmt.Errorf("expected type %T, got %T", msg, v)
	}

	plain, err := mgr.Decrypt(ctx, msg.ContentBytes(), msg.Author())
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
	rcpts      []ssb.Ref
	// ...
}

// TODO: If one of the refs is not a feedref, set box2
func WithRecipients(rcpts ...ssb.Ref) EncryptOption {
	return func(cfg *encConfig) error {
		cfg.rcpts = append(cfg.rcpts, rcpts...)
		return nil
	}
}

var defaultEncConfig encConfig

func init() {
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
			bxr   = box.NewBoxer(rand.Reader)
			rcpts = make([]*ssb.FeedRef, len(cfg.rcpts))
			ok    bool
		)

		for i := range cfg.rcpts {
			rcpts[i], ok = cfg.rcpts[i].(*ssb.FeedRef)
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
			keySlice = make(keys.Keys, 0, len(cfg.rcpts))
			ks       keys.Keys
			keyType  keys.Type
			keyID    keys.ID
		)

		for _, rcpt := range cfg.rcpts {
			switch ref := rcpt.(type) {
			case *ssb.FeedRef:
				keyType = "shared-secret-by-feeds"
				keyID = keys.ID(sortAndConcat(mgr.author.ID, ref.ID))
			case *ssb.MessageRef:
				// TODO: maybe verify this is a group message?
				keyType = "group-shared-secret"
				keyID = keys.ID(sortAndConcat(ref.Hash)) // actually just copy
			}

			ks, err = mgr.keymgr.GetKeys(ctx, keyType, keyID)
			if err != nil {
				return nil, errors.Wrapf(err, "could not get key for recipient %s", rcpt.Ref())
			}

			box2ks := make(keys.Keys, len(ks))
			for i := range box2ks {
				box2ks[i] = keys.Key(ks[i])
			}
			keySlice = append(keySlice, box2ks...)
		}

		// then, encrypt message
		bxr := box2.NewBoxer(rand.Reader)
		boxmsg, err := bxr.Encrypt(nil, contentBs, nil, keySlice)
		if err != nil {
			return nil, errors.Wrap(err, "error encrypting message (box1)")
		}

		ctxt = boxmsg.Raw
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

func (mgr *Manager) Decrypt(ctx context.Context, ctxt []byte, author *ssb.FeedRef, opts ...EncryptOption) (interface{}, error) {
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
		ks      keys.Keys
		keyType keys.Type
		keyID   keys.ID
		plain   []byte
		v       interface{}
	)

	switch cfg.boxVersion {
	case 1: // case box1
		keyPair := ssb.KeyPair{Id: mgr.author}

		// read secret DH key from database
		keyType = "box1-ed25519-sec"
		keyID = sortAndConcat(mgr.author.ID)

		ks, err = mgr.keymgr.GetKeys(ctx, keyType, keyID)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get key for recipient %s", mgr.author.Ref())
		}

		if len(ks) < 1 {
			return nil, fmt.Errorf("no cv25519 secret for feed id %s", mgr.author)
		}
		copy(keyPair.Pair.Secret[:], ks[0])
		copy(keyPair.Pair.Public[:], mgr.author.ID)

		// try decrypt
		bxr := box.NewBoxer(rand.Reader)

		plain, err = bxr.Decrypt(&keyPair, []byte(ctxt))
		if err != nil {
			return nil, errors.Wrap(err, "could not decrypt")
		}

		err = json.Unmarshal(plain, &v)
	case 2: // case box2
		// fetch feed2feed shared key
		keyType = "shared-secret-by-feeds"
		keyID = sortAndConcat(mgr.author.ID, author.ID)

		ks, err = mgr.keymgr.GetKeys(ctx, keyType, keyID)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get key for author %s", author.Ref())
		}

		// fetch groups author is member of
		// TODO: we don't track this yet

		// fetch group keys
		// TODO: we don't know which group keys to fetch yet

		// try decrypt
	default:
		return nil, fmt.Errorf("expected string to end with either %q or %q", ".box", ".box2")
	}

	return v, err
}

func (mgr *Manager) ResponsePlan(msg ssb.Message) (msgkit.Plan, error) {
	// determine box version
	// determine candidate keys
	// try decrypt
	// if ok, return plan to respond to message
	panic("not implemented")
	return nil, nil
}
