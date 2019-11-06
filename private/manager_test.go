package private

import (
	"context"
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/agl/ed25519/extra25519"
	"github.com/keks/testops"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
	"modernc.org/kv"

	"go.cryptoscope.co/librarian"
	libmkv "go.cryptoscope.co/librarian/mkv"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
)

func TestManager(t *testing.T) {
	km := &keys.Manager{
		Index: newMemIndex(keys.Keys{}),
	}

	type testcase struct {
		name    string
		msg     []byte
		sender  *ssb.FeedRef
		rcpts   []ssb.Ref
		encOpts []EncryptOption
	}

	var (
		alice = newIdentity(t, "Alice", km)
		bob   = newIdentity(t, "Bob", km)
	)

	populateKeyStore(t, km, alice, bob)

	type testStruct struct {
		Hello bool `json:"hello"`
	}

	var (
		ctxt []byte
		msg  interface{}

		msgs = []interface{}{
			"plainStringLikeABlob",
			[]int{1, 2, 3, 4, 5},
			map[string]interface{}{"some": 1, "msg": "here"},
			testStruct{true},
			testStruct{false},
			map[string]interface{}{"hello": false},
			map[string]interface{}{"hello": true},
			json.RawMessage("omg this isn't even valid json"),
		}
	)

	tcs2 := []testops.TestCase{
		testops.TestCase{
			Name: "alice->alice, string",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[0],
					Recipients: []ssb.Ref{alice.ref},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.ref,
					Ciphertext: &ctxt,

					Message: &msg,

					ExpMessage: msgs[0],
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice, struct",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[3],
					Recipients: []ssb.Ref{alice.ref},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.ref,
					Ciphertext: &ctxt,

					Message: &testStruct{},

					ExpMessage: msgs[3],
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice+bob, slice",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[1],
					Recipients: []ssb.Ref{alice.ref, bob.ref},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.ref,
					Ciphertext: &ctxt,

					Message: &[]int{},

					ExpMessage: msgs[1],
				},
				OpManagerDecrypt{
					Manager:    bob.manager,
					Sender:     alice.ref,
					Ciphertext: &ctxt,

					Message: &[]int{},

					ExpMessage: msgs[1],
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice+bob, slice, box2",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[1],
					Recipients: []ssb.Ref{alice.ref, bob.ref},
					Options:    []EncryptOption{WithBox2()},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.ref,
					Ciphertext: &ctxt,
					Options:    []EncryptOption{WithBox2()},

					Message: &[]int{},

					ExpMessage: msgs[1],
				},
				OpManagerDecrypt{
					Manager:    bob.manager,
					Sender:     alice.ref,
					Ciphertext: &ctxt,
					Options:    []EncryptOption{WithBox2()},

					Message: &[]int{},

					ExpMessage: msgs[1],
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
}

func newMemIndex(tipe interface{}) librarian.SeqSetterIndex {
	db, err := kv.CreateMem(&kv.Options{})
	if err != nil {
		// sorryyy
		// but this is for testing only and unlikely to fail
		panic(err)
	}

	return libmkv.NewIndex(db, tipe)
}

type testIdentity struct {
	*ssb.KeyPair

	name    string
	manager *Manager
	ref     *ssb.FeedRef
}

var idCount int64

func newIdentity(t *testing.T, name string, km *keys.Manager) testIdentity {
	var (
		id  = testIdentity{name: name}
		err error
	)

	id.KeyPair, err = ssb.NewKeyPair(nil)
	require.NoError(t, err)

	id.ref = id.Id
	t.Logf("%s is %s", name, id.ref.Ref())

	id.manager = &Manager{
		author: id.Id,
		keymgr: km,
		rand:   rand.New(rand.NewSource(idCount)),
	}

	idCount++

	return id
}

func populateKeyStore(t *testing.T, km *keys.Manager, ids ...testIdentity) {
	type keySpec struct {
		Type keys.Type
		ID   keys.ID
		Key  keys.Key
	}

	// TODO make these type strings constants
	specs := make([]keySpec, 0, (len(ids)+2)*len(ids))

	var (
		cvSecs = make([][32]byte, len(ids))
		cvPubs = make([][32]byte, len(ids))
		shared = make([][][32]byte, len(ids))
	)

	for i := range ids {
		specs = append(specs, keySpec{
			"box1-ed25519-sec",
			keys.ID(ids[i].Id.ID),
			keys.Key(ids[i].Pair.Secret[:]),
		})

		extra25519.PrivateKeyToCurve25519(&cvSecs[i], &ids[i].Pair.Secret)
		extra25519.PublicKeyToCurve25519(&cvPubs[i], &ids[i].Pair.Public)

		specs = append(specs, keySpec{
			"box1-cv25519-sec",
			keys.ID(ids[i].Id.ID),
			keys.Key(cvSecs[i][:]),
		})
	}

	for i := range ids {
		shared[i] = make([][32]byte, len(ids))
		for j := range ids {
			var (
				shrd = &shared[i][j]
				pub  = &cvPubs[i]
				sec  = &cvSecs[j]
			)
			box.Precompute(shrd, pub, sec)

			specs = append(specs, keySpec{
				"box2-shared-by-feeds",
				sortAndConcat(keys.ID(ids[i].Id.ID), keys.ID(ids[j].Id.ID)),
				keys.Key(shared[i][j][:]),
			})
		}

	}

	var err error

	ctx := context.TODO()

	for _, spec := range specs {
		t.Logf("adding key %s - %x", spec.Type, spec.ID)
		err = km.AddKey(ctx, spec.Type, spec.ID, spec.Key)
		require.NoError(t, err)
	}

}

/** Legacy tests, should be covered by the one above
func TestManagerVeryLegacy(t *testing.T) {
	var (
		ctx = context.Background()
		r   = require.New(t)

		alice = newIdentity(t, "Alice", &keys.Manager{
			Index: newMemIndex(keys.Keys{}),
		})

		tmsgs = [][]byte{
			[]byte(`[1,2,3,4,5]`),
			[]byte(`{"some": 1, "msg": "here"}`),
			[]byte(`{"hello": true}`),
			[]byte(`"plainStringLikeABlob"`),
			[]byte(`{"hello": false}`),
			[]byte(`{"hello": true}`),
		}
	)

	populateKeyStore(t, alice.manager.keymgr, alice)

	for i, msg := range tmsgs {
		ctxt, err := alice.manager.Encrypt(ctx, msg, WithRecipients(alice.Id))
		r.NoError(err, "failed to create ciphertext %d", i)

		var outStr string
		err = alice.manager.Decrypt(ctx, &outStr, ctxt, alice.Id)
		r.NoError(err, "should decrypt my message %d", i)
		out, _ := base64.StdEncoding.DecodeString(outStr)
		r.True(bytes.Equal(out, msg), "msg decrypted not equal %d", i)
	}

}

func TestManagerLegacy(t *testing.T) {
	ctx := context.Background()

	km := &keys.Manager{
		Index: newMemIndex(keys.Keys{}),
	}

	type testcase struct {
		name    string
		msg     []byte
		sender  *ssb.FeedRef
		rcpts   []ssb.Ref
		encOpts []EncryptOption
	}

	var (
		alice = newIdentity(t, "Alice", km)
	)

	populateKeyStore(t, alice.manager.keymgr, alice)

	var tcs = []testcase{
		{
			name:   "alice sends a message to herself",
			msg:    []byte(`"hello me!"`),
			sender: alice.Id,
			rcpts:  []ssb.Ref{alice.Id},
		},
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

				var outStr string

				// TODO: figure out how to pass in the recipients.
				//       maybe don't pass them in as options??
				err := rcptMgr.Decrypt(ctx, &outStr, ctxt, tc.sender)
				r.NoError(err, "should decrypt my message")

				out, _ := base64.StdEncoding.DecodeString(outStr)
				r.True(bytes.Equal(out, tc.msg), "msg decrypted not equal")
			}
		}
	}

	for _, tc := range tcs {
		t.Run(tc.name, runner(tc))
	}
}
**/
