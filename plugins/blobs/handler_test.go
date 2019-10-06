// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/plugins/test"
	"go.cryptoscope.co/ssb/repo"
)

func TestReplicate(t *testing.T) {
	r := require.New(t)

	srcRepo, srcPath := test.MakeEmptyPeer(t)
	dstRepo, dstPath := test.MakeEmptyPeer(t)

	srcBS, err := repo.OpenBlobStore(srcRepo)
	r.NoError(err, "error src opening blob store")

	srcLog, _ := logtest.KitLogger("alice/src", t)
	//srcLog = kitlog.With(kitlog.NewSyncLogger(kitlog.NewLogfmtLogger(os.Stderr)), "node", "src/alice")
	//srcLog := logging.Logger("alice/src")
	srcWM := blobstore.NewWantManager(srcLog, srcBS)

	dstBS, err := repo.OpenBlobStore(dstRepo)
	r.NoError(err, "error dst opening blob store")
	dstLog, _ := logtest.KitLogger("bob/dst", t)
	//dstLog = kitlog.With(kitlog.NewSyncLogger(kitlog.NewLogfmtLogger(os.Stderr)), "node", "dst/bob")
	//dstLog := logging.Logger("bob/dst")
	dstWM := blobstore.NewWantManager(dstLog, dstBS)

	// do the dance
	pkr1, pkr2, _, serve := test.PrepareConnectAndServe(t, srcRepo, dstRepo)

	pi1 := New(srcLog, srcBS, srcWM)
	pi2 := New(dstLog, dstBS, dstWM)

	ref, err := srcBS.Put(strings.NewReader("testString"))
	r.NoError(err, "error putting blob at src")

	err = dstWM.Want(ref)
	r.NoError(err, "error wanting blob at dst")

	finish := make(chan func())
	done := make(chan struct{})
	dstBS.Changes().Register(
		luigi.FuncSink(
			func(ctx context.Context, v interface{}, err error) error {
				n := v.(ssb.BlobStoreNotification)
				if n.Op == ssb.BlobStoreOpPut {
					if n.Ref.Ref() == ref.Ref() {
						t.Log("received correct blob")
						(<-finish)()
						close(done)
					} else {
						t.Error("received unexpected blob:", n.Ref.Ref())
					}
				}
				return nil
			}))

	// serve
	rpc1 := muxrpc.Handle(pkr1, pi1.Handler())
	rpc2 := muxrpc.Handle(pkr2, pi2.Handler())

	finish <- serve(rpc1, rpc2)

	<-done
	t.Log("after blobs")

	// check data ended up on the target
	blob, err := dstBS.Get(ref)
	r.NoError(err, "failed to get blob")
	r.NotNil(blob, "returned blob is nil")

	blobStr, err := ioutil.ReadAll(blob)
	r.NoError(err, "failed to read blob")

	r.Equal("testString", string(blobStr), "blob value mismatch")

	/* TODO test transcript here

	spec := muxtest.MergeTranscriptSpec(
		muxtest.UniqueMatchTranscriptSpec(
			muxtest.MergePacketSpec(
				muxtest.CallPacketSpec(true, true, muxrpc.Method{"blobs", "createWants"}, "source"),
				muxtest.ReqPacketSpec(1),
				muxtest.DirPacketSpec(muxtest.DirOut),
			),
		),
		muxtest.UniqueMatchTranscriptSpec(
			muxtest.MergePacketSpec(
				muxtest.CallPacketSpec(true, true, muxrpc.Method{"blobs", "createWants"}, "source"),
				muxtest.ReqPacketSpec(1),
				muxtest.DirPacketSpec(muxtest.DirIn),
			),
		),

		/* TODO currently fails.
		muxtest.MatchCountTranscriptSpec(
			muxtest.MergePacketSpec(
				muxtest.ReqPacketSpec(-1),
				muxtest.BodyPacketSpec(
					muxtest.EqualBodySpec(codec.Body("{}")),
				),
			),
		0),

	)


		for i, dpkt := range ts.Get() {
			t.Logf("%3d: dir:%6s %v", i, dpkt.Dir, dpkt.Packet)
		}
		spec(t, ts)
	*/

	if !t.Failed() {
		os.RemoveAll(dstPath)
		os.RemoveAll(srcPath)
	}
}
