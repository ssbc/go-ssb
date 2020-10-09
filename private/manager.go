package private

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/pkg/errors"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/ed25519"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
	"go.cryptoscope.co/ssb/private/box"
	"go.cryptoscope.co/ssb/private/box2"
)

type Manager struct {
	// log    margaret.Log // Q: ???? which log is this?!
	publog ssb.Publisher

	author *refs.FeedRef

	keymgr *keys.Store
	rand   io.Reader
}

func NewManager(author *refs.FeedRef, publishLog ssb.Publisher, km *keys.Store) *Manager {
	return &Manager{
		author: author,
		publog: publishLog,
		keymgr: km,
		rand:   rand.Reader,
	}
}

func (mgr *Manager) EncryptBox1(content []byte, rcpts ...*refs.FeedRef) ([]byte, error) {
	bxr := box.NewBoxer(mgr.rand)
	ctxt, err := bxr.Encrypt(content, rcpts...)
	return ctxt, errors.Wrap(err, "error encrypting message (box1)")
}

func (mgr *Manager) EncryptBox2(content []byte, prev *refs.MessageRef, recpts []refs.Ref) ([]byte, error) {

	// first, look up keys
	var (
		ks        keys.Recipients
		keyScheme keys.KeyScheme
		keyID     keys.ID
	)

	for _, rcpt := range recpts {
		switch ref := rcpt.(type) {
		case *refs.FeedRef:
			keyScheme = keys.SchemeDiffieStyleConvertedED25519
			keyID = keys.ID(sortAndConcat(mgr.author.ID, ref.ID))
		case *refs.MessageRef:
			// TODO: maybe verify this is a group message?
			keyScheme = keys.SchemeLargeSymmetricGroup
			keyID = keys.ID(sortAndConcat(ref.Hash)) // actually just copy
		}

		ks_, err := mgr.keymgr.GetKeys(keyScheme, keyID)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get key for recipient %s", rcpt.Ref())
		}

		ks = append(ks, ks_...)
	}

	// then, encrypt message
	bxr := box2.NewBoxer(mgr.rand)
	ctxt, err := bxr.Encrypt(content, mgr.author, prev, ks)
	return ctxt, errors.Wrap(err, "error encrypting message (box1)")

}
func (mgr *Manager) DecryptBox1(ctxt []byte, author *refs.FeedRef) ([]byte, error) {
	keyPair := ssb.KeyPair{Id: mgr.author}
	keyPair.Pair.Secret = make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	keyPair.Pair.Public = make(ed25519.PublicKey, ed25519.PublicKeySize)

	// read secret DH key from database
	keyScheme := keys.SchemeDiffieStyleConvertedED25519
	keyID := sortAndConcat(mgr.author.ID)

	ks, err := mgr.keymgr.GetKeys(keyScheme, keyID)
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
	plain, err := bxr.Decrypt(&keyPair, []byte(ctxt))
	return plain, errors.Wrap(err, "could not decrypt")
}

func (mgr *Manager) DecryptBox2(ctxt []byte, author *refs.FeedRef, prev *refs.MessageRef) ([]byte, error) {
	// assumes 1:1 pm
	// fetch feed2feed shared key
	keyScheme := keys.SchemeDiffieStyleConvertedED25519
	keyID := sortAndConcat(mgr.author.ID, author.ID)
	var allKeys keys.Recipients
	if ks, err := mgr.keymgr.GetKeys(keyScheme, keyID); err == nil {
		allKeys = append(allKeys, ks...)
	}

	// try my groups
	keyScheme = keys.SchemeLargeSymmetricGroup
	keyID = sortAndConcat(mgr.author.ID, mgr.author.ID)
	if ks, err := mgr.keymgr.GetKeys(keyScheme, keyID); err == nil {
		allKeys = append(allKeys, ks...)
	}

	// try decrypt
	bxr := box2.NewBoxer(mgr.rand)
	plain, err := bxr.Decrypt([]byte(ctxt), author, prev, allKeys)
	return plain, errors.Wrap(err, "could not decrypt")

}
