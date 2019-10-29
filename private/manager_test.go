package private

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/agl/ed25519/extra25519"
	"github.com/keks/testops"
	"github.com/stretchr/testify/require"
	"modernc.org/kv"

	"go.cryptoscope.co/librarian"
	libmkv "go.cryptoscope.co/librarian/mkv"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
)

func newMemIndex(tipe interface{}) librarian.SeqSetterIndex {
	db, err := kv.CreateMem(&kv.Options{})
	if err != nil {
		// sorryyy
		// but this is for testing only and unlikely to fail
		panic(err)
	}

	return libmkv.NewIndex(db, tipe)
}

func TestManager(t *testing.T) {
	ctx := context.Background()
	r := require.New(t)

	km := &keys.Manager{
		Index: newMemIndex(keys.Keys{}),
	}

	var tmsgs = [][]byte{
		[]byte(`[1,2,3,4,5]`),
		[]byte(`{"some": 1, "msg": "here"}`),
		[]byte(`{"hello": true}`),
		[]byte(`"plainStringLikeABlob"`),
		[]byte(`{"hello": false}`),
		[]byte(`{"hello": true}`),
	}

	type testcase struct {
		name    string
		msg     []byte
		sender  *ssb.FeedRef
		rcpts   []ssb.Ref
		encOpts []EncryptOption
	}

	type testIdentity struct {
		*ssb.KeyPair

		name    string
		manager *Manager
		ref     *ssb.FeedRef
	}

	newIdentity := func(name string) testIdentity {
		var (
			id  = testIdentity{name: name}
			err error
		)

		id.KeyPair, err = ssb.NewKeyPair(nil)
		r.NoError(err)

		id.ref = id.Id
		t.Logf("%s is %s", name, id.ref.Ref())

		id.manager = &Manager{
			author: id.Id,
			keymgr: km,
		}

		return id
	}

	var (
		alice = newIdentity("Alice")
		bob   = newIdentity("Bob")
	)

	var tcs = []testcase{
		{
			name:   "alice sends a message to herself",
			msg:    []byte(`"hello me!"`),
			sender: alice.Id,
			rcpts:  []ssb.Ref{alice.Id},
		},
	}

	var cvSecAlice, cvSecBob [32]byte
	extra25519.PrivateKeyToCurve25519(&cvSecAlice, &alice.Pair.Secret)
	extra25519.PrivateKeyToCurve25519(&cvSecBob, &bob.Pair.Secret)

	type keySpec struct {
		Type keys.Type
		ID   keys.ID
		Key  keys.Key
	}

	// TODO make these type strings constants
	specs := []keySpec{
		keySpec{"box1-ed25519-sec", keys.ID(alice.Id.ID), keys.Key(alice.Pair.Secret[:])},
		keySpec{"box1-ed25519-sec", keys.ID(bob.Id.ID), keys.Key(bob.Pair.Secret[:])},
		keySpec{"box1-cv25519-sec", keys.ID(alice.Id.ID), keys.Key(cvSecAlice[:])},
		keySpec{"box1-cv25519-sec", keys.ID(bob.Id.ID), keys.Key(cvSecBob[:])},
	}

	var err error

	for _, spec := range specs {
		err = km.AddKey(ctx, spec.Type, spec.ID, spec.Key)
		r.NoError(err)
	}

	mgr := &Manager{
		author: alice.Id,
		keymgr: km,
	}

	runner := func(tc testcase) func(*testing.T) {
		return func(t *testing.T) {
			r := require.New(t)

			senderMgr := &Manager{
				author: tc.sender,
				keymgr: km,
			}

			// add recipients option
			encOpts := make([]EncryptOption, len(tc.encOpts)+1)
			encOpts[0] = WithRecipients(tc.rcpts...)
			copy(encOpts[1:], tc.encOpts)

			t.Log(tc.msg)
			t.Log(encOpts)

			ctxt, err := senderMgr.Encrypt(ctx, tc.msg, encOpts...)
			r.NoError(err, "failed to create ciphertext")

			for _, rcpt_ := range tc.rcpts {
				// TODO: handle other types as well
				rcpt := rcpt_.(*ssb.FeedRef)

				rcptMgr := &Manager{
					author: rcpt,
					keymgr: km,
				}

				// TODO: figure out how to pass in the recipients.
				//       maybe don't pass them in as options??
				out, err := rcptMgr.Decrypt(ctx, ctxt, tc.sender)
				r.NoError(err, "should decrypt my message")

				out, _ = base64.StdEncoding.DecodeString(out.(string))
				r.True(bytes.Equal(out.([]byte), tc.msg), "msg decrypted not equal")
			}
		}
	}

	for _, tc := range tcs {
		t.Run(tc.name, runner(tc))
	}

	var (
		ctxt, msg []byte
		wat       = []byte("wat")
	)

	tcs2 := []testops.TestCase{
		testops.TestCase{
			Name: "simple encrypt and decrypt",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &wat,
					Recipients: []ssb.Ref{alice.ref},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.ref,
					Ciphertext: &ctxt,

					Message: &msg,

					ExpMessage: []byte("wat"),
				},
			},
		},
	}

	testops.Run(t, []testops.Env{testops.Env{
		Name: "private.Manager",
		Func: func(tc testops.TestCase) (func(*testing.T), error) {
			return tc.Runner(nil), nil
		},
	}}, tcs2)

	for i, msg := range tmsgs {
		ctxt, err := mgr.Encrypt(ctx, msg, WithRecipients(alice.Id))
		r.NoError(err, "failed to create ciphertext %d", i)

		out, err := mgr.Decrypt(ctx, ctxt, alice.Id)
		r.NoError(err, "should decrypt my message %d", i)
		out, _ = base64.StdEncoding.DecodeString(out.(string))
		r.True(bytes.Equal(out.([]byte), msg), "msg decrypted not equal %d", i)
	}
}
