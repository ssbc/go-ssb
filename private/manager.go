package private

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
	"go.cryptoscope.co/ssb/message/msgkit"
	"go.cryptoscope.co/ssb/private/box2"
)

type Manager struct {
	log    margaret.Log
	author *ssb.FeedRef

	keymgr keys.Manager
}

func (mgr *Manager) Pour(ctx context.Context, v interface{}) error {
	msg, ok := v.(ssb.Message)
	if !ok {
		return fmt.Errorf("expected type %T, got %T", msg, v)
	}

	plain, err := mgr.Decrypt(msg)
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
			rcpts = make([]*ssb.FeedRef, len(cfg.rcpts))
			ok    bool
		)

		for i := range cfg.rcpts {
			rcpts[i], ok = cfg.rcpts[i].(*ssb.FeedRef)
			if !ok {
				return nil, fmt.Errorf("box1 can only send to feed ids")
			}
		}

		ctxt, err = Box(contentBs, rcpts...)
		if err != nil {
			return nil, errors.Wrap(err, "error encrypting message (box1)")
		}
	case 2:
		// first, look up keys
		var (
			keySlice = make([]box2.Key, 0, len(cfg.rcpts))
			ks       keys.Keys
			keyType  keys.Type
			keyID    keys.ID
		)

		for _, rcpt := range cfg.rcpts {
			switch ref := rcpt.(type) {
			case *ssb.FeedRef:
				keyType = "shared-secret-by-feeds"
				keyID = make([]byte, len(mgr.author.ID)+len(ref.ID))
				n := copy(keyID, mgr.author.ID)
				copy(keyID[n:], ref.ID)
			case *ssb.MessageRef:
				// TODO: maybe verify this is a group message?
				keyType = "group-shared-secret"
				keyID = make([]byte, len(ref.Hash))
				copy(keyID, ref.Hash)
			}

			ks, err = mgr.keymgr.GetKeys(ctx, keyType, keyID)
			if err != nil {
				return nil, errors.Wrapf(err, "could not get key for recipient %s", rcpt)
			}

			box2ks := make([]box2.Key, len(ks))
			for i := range box2ks {
				box2ks[i] = box2.Key(ks[i])
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

func (mgr *Manager) Decrypt(msg ssb.Message) (interface{}, error) {
	panic("not implemented")
	return nil, nil
}

func (mgr *Manager) ResponsePlan(msg ssb.Message) (msgkit.Plan, error) {
	panic("not implemented")
	return nil, nil
}
