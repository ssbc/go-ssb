// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	kitlog "go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/internal/broadcasts"
	"go.cryptoscope.co/ssb/plugins/test"
	"go.cryptoscope.co/ssb/repo"
)

func TestReplicate(t *testing.T) {
	r := require.New(t)

	srcRepo, srcPath := test.MakeEmptyPeer(t)
	dstRepo, dstPath := test.MakeEmptyPeer(t)

	srcKP, err := repo.DefaultKeyPair(srcRepo, refs.RefAlgoFeedSSB1)
	r.NoError(err)
	dstKP, err := repo.DefaultKeyPair(dstRepo, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	srcBS, err := repo.OpenBlobStore(srcRepo)
	r.NoError(err, "error src opening blob store")

	srcLog := kitlog.With(kitlog.NewSyncLogger(kitlog.NewLogfmtLogger(os.Stderr)), "node", "src/alice")
	srcWM := blobstore.NewWantManager(srcBS, blobstore.WantWithLogger(srcLog))

	dstBS, err := repo.OpenBlobStore(dstRepo)
	r.NoError(err, "error dst opening blob store")

	dstLog := kitlog.With(kitlog.NewSyncLogger(kitlog.NewLogfmtLogger(os.Stderr)), "node", "dst/bob")
	dstWM := blobstore.NewWantManager(dstBS, blobstore.WantWithLogger(dstLog))

	// do the dance
	pkr1, pkr2, _ := test.PrepareConnectAndServe(t, srcRepo, dstRepo)

	pi1 := New(srcLog, srcKP.ID(), srcBS, srcWM)
	pi2 := New(dstLog, dstKP.ID(), dstBS, dstWM)

	// serve
	var rpc1, rpc2 muxrpc.Endpoint

	var (
		justBlobsManifest = json.RawMessage(`{"blobs": {"get": "source", "createWants": "source"}}`)

		servedGroup, initedGroup sync.WaitGroup
		err1, err2               error
	)
	initedGroup.Add(2)
	servedGroup.Add(2)
	go func() {
		var tw1 = testManifestWrapper{root: pi1.Handler(), manifest: justBlobsManifest}
		rpc1 = muxrpc.Handle(pkr1, tw1)
		initedGroup.Done()
		err1 = rpc1.(muxrpc.Server).Serve()
		servedGroup.Done()
	}()

	go func() {
		var tw2 = testManifestWrapper{root: pi2.Handler(), manifest: justBlobsManifest}
		rpc2 = muxrpc.Handle(pkr2, tw2)
		initedGroup.Done()
		err2 = rpc2.(muxrpc.Server).Serve()
		servedGroup.Done()
	}()

	wait := func() {
		initedGroup.Wait()
		r.NoError(rpc1.Terminate())
		r.NoError(rpc2.Terminate())

		servedGroup.Wait()
		r.NoError(err1, "rpc1 serve err")
		r.NoError(err2, "rpc2 serve err")
	}
	t.Log("serving")

	ref, err := srcBS.Put(strings.NewReader("testString"))
	r.NoError(err, "error putting blob at src")

	err = dstWM.Want(ref)
	r.NoError(err, "error wanting blob at dst")

	t.Log("blob wanted")

	done := make(chan struct{})
	dstBS.Register(broadcasts.BlobStoreFuncEmitter(func(n ssb.BlobStoreNotification) error {
		t.Log("blob notification", n)

		if n.Op == ssb.BlobStoreOpPut {
			if n.Ref.Sigil() == ref.Sigil() {
				t.Log("received correct blob")
				wait()
				close(done)
			} else {
				t.Error("received unexpected blob:", n.Ref.Sigil())
			}
		}
		return nil
	}))

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

type testManifestWrapper struct {
	manifest json.RawMessage
	root     muxrpc.Handler
}

func (w testManifestWrapper) Handled(m muxrpc.Method) bool {
	if m.String() == "manifest" {
		return true
	}
	return w.root.Handled(m)
}

func (w testManifestWrapper) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	w.root.HandleConnect(ctx, edp)
}
func (w testManifestWrapper) HandleCall(ctx context.Context, req *muxrpc.Request) {
	if req.Method[0] == "manifest" {
		err := req.Return(ctx, w.manifest)
		if err != nil {
			fmt.Println("manifest return error:", err)
		}
		return
	}

	w.root.HandleCall(ctx, req)
}
