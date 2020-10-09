package private

import (
	"math/rand"
	"testing"

	"github.com/keks/testops"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	libmkv "go.cryptoscope.co/librarian/mkv"
	"golang.org/x/crypto/nacl/box"
	"modernc.org/kv"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/extra25519"
	"go.cryptoscope.co/ssb/keys"
	refs "go.mindeco.de/ssb-refs"
)

func TestManager(t *testing.T) {
	ks := &keys.Store{
		Index: newMemIndex(keys.Keys{}),
	}

	type testcase struct {
		name   string
		msg    []byte
		sender *refs.FeedRef
		rcpts  []refs.Ref
	}

	var (
		alice = newIdentity(t, "Alice", ks)
		bob   = newIdentity(t, "Bob", ks)
	)

	populateKeyStore(t, ks, alice, bob)

	var ctxt []byte

	tcs2 := []testops.TestCase{
		testops.TestCase{
			Name: "alice->alice",
			Ops: []testops.Op{
				OpManagerEncryptBox1{
					Manager:    alice.manager,
					Recipients: []*refs.FeedRef{alice.Id},

					Ciphertext: &ctxt,
				},
				OpManagerDecryptBox1{
					Manager:    alice.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice+bob",
			Ops: []testops.Op{
				OpManagerEncryptBox1{
					Manager: alice.manager,

					Recipients: []*refs.FeedRef{alice.Id, bob.Id},

					Ciphertext: &ctxt,
				},
				OpManagerDecryptBox1{
					Manager:    alice.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,
				},
				OpManagerDecryptBox1{
					Manager:    bob.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice+bob, box2",
			Ops: []testops.Op{
				OpManagerEncryptBox2{
					Manager:    alice.manager,
					Recipients: []refs.Ref{alice.Id, bob.Id},
					Ciphertext: &ctxt,
				},
				OpManagerDecryptBox2{
					Manager:    alice.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,
				},
				OpManagerDecryptBox2{
					Manager:    bob.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,
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
		// this is for testing only and unlikely to fail
		panic(err)
	}

	return libmkv.NewIndex(db, tipe)
}

type testIdentity struct {
	*ssb.KeyPair

	name    string
	manager *Manager
}

var idCount int64

func newIdentity(t *testing.T, name string, km *keys.Store) testIdentity {
	var (
		id  = testIdentity{name: name}
		err error
	)

	rand := rand.New(rand.NewSource(idCount))

	id.KeyPair, err = ssb.NewKeyPair(rand)
	require.NoError(t, err)

	t.Logf("%s is %s", name, id.Id.Ref())

	id.manager = &Manager{
		author: id.Id,
		keymgr: km,
		rand:   rand,
	}

	idCount++

	return id
}

func populateKeyStore(t *testing.T, km *keys.Store, ids ...testIdentity) {
	type keySpec struct {
		Scheme keys.KeyScheme
		ID     keys.ID
		Key    keys.Key
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
			keys.SchemeDiffieStyleConvertedED25519,
			keys.ID(ids[i].Id.ID),
			keys.Key(ids[i].Pair.Secret[:]),
		})

		extra25519.PrivateKeyToCurve25519(&cvSecs[i], ids[i].Pair.Secret)
		extra25519.PublicKeyToCurve25519(&cvPubs[i], ids[i].Pair.Public)

		specs = append(specs, keySpec{
			keys.SchemeDiffieStyleConvertedED25519,
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
				keys.SchemeDiffieStyleConvertedED25519,
				sortAndConcat(keys.ID(ids[i].Id.ID), keys.ID(ids[j].Id.ID)),
				keys.Key(shared[i][j][:]),
			})
		}

	}

	var err error

	for _, spec := range specs {
		t.Logf("adding key %s - %x", spec.Scheme, spec.ID)
		err = km.AddKey(spec.Scheme, spec.ID, spec.Key)
		require.NoError(t, err)
	}

}
