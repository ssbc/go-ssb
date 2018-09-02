package blobs

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	//kitlog "github.com/go-kit/kit/log"
	"github.com/cryptix/go/logging/logtest"
	//"github.com/cryptix/go/logging"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
	"go.cryptoscope.co/sbot/plugins/test"
)

func TestReplicate(t *testing.T) {
	r := require.New(t)

	srcRepo, srcPath := test.MakeEmptyPeer(t)
	dstRepo, dstPath := test.MakeEmptyPeer(t)

	srcBS := srcRepo.BlobStore()
	srcLog, _ := logtest.KitLogger("alice/src", t)
	//srcLog = kitlog.With(kitlog.NewSyncLogger(kitlog.NewLogfmtLogger(os.Stderr)), "node", "src/alice")
	//srcLog := logging.Logger("alice/src")
	srcWM := blobstore.NewWantManager(srcLog, srcBS)

	dstBS := dstRepo.BlobStore()
	dstLog, _ := logtest.KitLogger("bob/dst", t)
	//dstLog = kitlog.With(kitlog.NewSyncLogger(kitlog.NewLogfmtLogger(os.Stderr)), "node", "dst/bob")
	//dstLog := logging.Logger("bob/dst")
	dstWM := blobstore.NewWantManager(dstLog, dstBS)

	// do the dance
	pkr1, pkr2, serve := test.PrepareConnectAndServe(t, srcRepo, dstRepo)

	pi1 := New(srcLog, srcBS, srcWM)
	pi2 := New(dstLog, dstBS, dstWM)

	ref, err := srcBS.Put(strings.NewReader("testString"))
	r.NoError(err, "error putting blob at src")

	err = dstWM.Want(ref)
	r.NoError(err, "error wanting blob at dst")

	var finish func()
	done := make(chan struct{})
	dstBS.Changes().Register(
		luigi.FuncSink(
			func(ctx context.Context, v interface{}, doClose bool) error {
				n := v.(sbot.BlobStoreNotification)
				if n.Op == sbot.BlobStoreOpPut {
					if n.Ref.Ref() == ref.Ref() {
						t.Log("received correct blob")
						finish()
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

	finish = serve(rpc1, rpc2)

	<-done
	t.Log("after blobs")

	// check data ended up on the target
	blob, err := dstBS.Get(ref)
	r.NoError(err, "failed to get blob")
	r.NotNil(blob, "returned blob is nil")

	blobStr, err := ioutil.ReadAll(blob)
	r.NoError(err, "failed to read blob")

	r.Equal("testString", string(blobStr), "blob value mismatch")

	if !t.Failed() {
		os.RemoveAll(dstPath)
		os.RemoveAll(srcPath)
	}
}
