// SPDX-License-Identifier: MIT

package blobstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/muxrpc/v2/codec"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/testutils"
)

func XTestWantManager(t *testing.T) {
	type testcase struct {
		blobs map[string]string // global key-value register of all blobs

		localBlobs     []string
		localLateBlobs []string

		localWants  map[string]int64
		remoteWants map[string]int64
	}

	tcs := []testcase{
		{
			blobs: map[string]string{
				"&6EcSI4cJOY9tNJ3CJQsO/KS3LYwr+3t0M50wupQFaxQ=.sha256": "ohai",
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256": "wat",
				"&ZR3jMW+ifnTWqd5hnrrGjjt4HpUn/dAMXvcUOx+lgbY=.sha256": "omg",
				"&47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=.sha256": "",
			},

			localBlobs:     []string{""},
			localLateBlobs: []string{"wat"}, // &8Ap4...
			localWants: map[string]int64{
				"&ZR3jMW+ifnTWqd5hnrrGjjt4HpUn/dAMXvcUOx+lgbY=.sha256": -1, // "omg"
				"&6EcSI4cJOY9tNJ3CJQsO/KS3LYwr+3t0M50wupQFaxQ=.sha256": -1, // "ohai"
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256": -2, // replicating the wat want
			},
			remoteWants: map[string]int64{
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256": -1, // "wat"
				"&47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=.sha256": -1, // ""
				"&6EcSI4cJOY9tNJ3CJQsO/KS3LYwr+3t0M50wupQFaxQ=.sha256": 4,  // "ohai"
			},
		},
	}

	mkStore := func(name string) (ssb.BlobStore, func() error, error) {
		var err error
		name = strings.Replace(name, "/", "_", -1)
		name, err = ioutil.TempDir("", name)
		if err != nil {
			return nil, nil, err
		}
		delBlobStore := func() error {
			return os.RemoveAll(name)
		}

		// remove debris of old test runs
		delBlobStore()

		store, err := New(name)
		return store, delBlobStore, err
	}

	mkTest := func(tc testcase) func(*testing.T) {
		return func(t *testing.T) {
			t.Log("Warning: currectly we only log errors that happen " +
				"when the node tries to fetch a blob the peer currently " +
				"has. TODO: Find a good way to make the test fail instead.")
			a := assert.New(t)
			r := require.New(t)

			bs, delBlobStore, err := mkStore(t.Name())
			r.NoError(err, "error creating want manager")
			defer func() {
				if !t.Failed() {
					a.NoError(delBlobStore(), "error deleting blob store directory")
				}
			}()

			log := testutils.NewRelativeTimeLogger(nil)
			wmgr := NewWantManager(bs, WantWithLogger(log))

			for _, str := range tc.localBlobs {
				br, err := bs.Put(strings.NewReader(str))
				r.NoError(err)
				r.NotNil(br)
			}

			for refStr, dist := range tc.localWants {
				ref, err := refs.ParseBlobRef(refStr)
				r.NoError(err, "error parsing ref %q", ref)

				a.NoError(wmgr.WantWithDist(ref, dist), "error wanting local ref")
			}

			for refStr := range tc.localWants {
				ref, err := refs.ParseBlobRef(refStr)
				r.NoError(err, "error parsing ref %q", ref)

				a.Equal(true, wmgr.Wants(ref), "expected want manager to want ref %q, but it doesn't", ref.Ref())
			}

			var wmsg WantMsg
			for refStr, dist := range tc.remoteWants {
				ref, err := refs.ParseBlobRef(refStr)
				r.NoError(err, "error parsing ref %q", ref)

				wmsg = append(wmsg, ssb.BlobWant{Ref: ref, Dist: dist})
			}

			outBuf := &bytes.Buffer{}
			out := muxrpc.NewTestSink(outBuf)

			ctx := context.Background()
			edp := &muxrpc.FakeEndpoint{
				SourceStub: func(ctx context.Context, enc muxrpc.RequestEncoding, method muxrpc.Method, args ...interface{}) (*muxrpc.ByteSource, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("expected one argument, got %v", len(args))
					}

					arg, ok := args[0].(GetWithSize)
					if !ok {
						return nil, fmt.Errorf("expected a string argument, got type %T", args[0])
					}

					sz, ok := tc.remoteWants[arg.Key.Ref()]
					if !ok || sz < 0 {
						return nil, ErrNoSuchBlob
					}

					data := tc.blobs[arg.Key.Ref()]

					return muxrpc.NewTestSource([]byte(data)), nil
				},
				RemoteStub: func() net.Addr {
					return &net.TCPAddr{Port: 666}
				},
			}
			proc := wmgr.CreateWants(ctx, out, edp)
			err = proc.Pour(ctx, wmsg)
			r.NoError(err, "error pouring first want message")

			sizeWants := func(strs []string) map[string]int64 {
				var (
					m   = make(map[string]int64)
					h   = sha256.New()
					ref = refs.BlobRef{
						Algo: refs.RefAlgoBlobSSB1,
					}
				)

				for _, str := range strs {
					h.Reset()
					h.Write([]byte(str))
					ref.Hash = h.Sum(nil)
					m[ref.Ref()] = int64(len(str))
				}

				return m
			}

			outReader := codec.NewReader(outBuf)

			// TODO this is pretty specific to the only test case defined above.
			// would be nice to generalize this further so we can add more cases.

			// should contain our wants and our size response to their want
			pkt1, err := outReader.ReadPacket()
			r.NoError(err)
			pkt2, err := outReader.ReadPacket()
			r.NoError(err)
			// r.Equal(2, len(outSlice), "output slice length mismatch (%v)", outSlice)

			// this should be our initial want list, but with more dist
			ourW := wmgr.(*wantManager)
			ourW.l.Lock()

			var wants map[string]int64
			err = json.Unmarshal(pkt1.Body, &wants)
			r.NoError(err)
			a.Equal(tc.localWants, wants, "map content mismatch (1)")

			// there is a small race somewhere here and this fails sometimes

			err = json.Unmarshal(pkt2.Body, &wants)
			r.NoError(err)
			a.Equal(sizeWants(tc.localBlobs), wants, "map content mismatch (2)")
			ourW.l.Unlock()

			for _, str := range tc.localLateBlobs {
				_, err := bs.Put(strings.NewReader(str))
				a.NoError(err, "error putting blob")
			}

			ourW.l.Lock()

			// should contain our wants and our size response to their want
			pkt3, err := outReader.ReadPacket()
			r.NoError(err)

			err = json.Unmarshal(pkt3.Body, &wants)
			r.NoError(err)
			a.Equal(sizeWants(tc.localBlobs), wants, "map content mismatch (3)")
			ourW.l.Unlock()
		}
	}

	for i, tc := range tcs {
		t.Run(fmt.Sprint(i), mkTest(tc))
	}
}
