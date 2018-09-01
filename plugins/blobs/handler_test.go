package blobs

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	//"github.com/cryptix/go/logging/logtest"
	"github.com/cryptix/go/logging"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/codec"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
	"go.cryptoscope.co/sbot/repo"
	"go.cryptoscope.co/secretstream"
)

func loadTestDataPeer(t *testing.T, repopath string) repo.Interface {
	r := require.New(t)
	repo, err := repo.New(repopath)
	r.NoError(err, "failed to load testData repo")
	r.NotNil(repo.KeyPair())
	return repo
}

func makeEmptyPeer(t *testing.T) (repo.Interface, string) {
	r := require.New(t)
	dstPath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)
	dstRepo, err := repo.New(dstPath)
	r.NoError(err, "failed to create emptyRepo")
	r.NotNil(dstRepo.KeyPair())
	return dstRepo, dstPath
}

func prepareConnectAndServe(t *testing.T, alice, bob repo.Interface) (muxrpc.Packer, muxrpc.Packer, func(rpc1, rpc2 muxrpc.Endpoint) func()) {
	r := require.New(t)
	keyAlice := alice.KeyPair()
	keyBob := bob.KeyPair()

	p1, p2 := net.Pipe()

	//infoAlice, _ := logtest.KitLogger("alice", t)
	//infoBob, _ := logtest.KitLogger("bob", t)
	infoAlice := logging.Logger("alice/src")
	infoBob := logging.Logger("bob/dst")

	tc1 := &testConn{
		Reader: p1, WriteCloser: p1, conn: p1,
		local:  keyAlice.Pair.Public[:],
		remote: keyBob.Pair.Public[:],
	}
	tc2 := &testConn{
		Reader: p2, WriteCloser: p2, conn: p2,
		local:  keyBob.Pair.Public[:],
		remote: keyAlice.Pair.Public[:],
	}

	var conn1, conn2 net.Conn = tc1, tc2

	// logs every muxrpc packet
	if testing.Verbose() {
		conn1 = codec.WrapConn(infoAlice, conn1)
		conn2 = codec.WrapConn(infoBob, conn2)
	}

	return muxrpc.NewPacker(conn1), muxrpc.NewPacker(conn2), func(rpc1, rpc2 muxrpc.Endpoint) func() {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			err := rpc1.(muxrpc.Server).Serve(ctx)
			r.NoError(err, "rpc1 serve err")
			wg.Done()
		}()

		go func() {
			err := rpc2.(muxrpc.Server).Serve(ctx)
			r.NoError(err, "rpc2 serve err")
			wg.Done()
		}()

		return func() {
			cancel()

			r.NoError(rpc1.Terminate())
			r.NoError(rpc2.Terminate())

			wg.Wait()
		}
	}
}

func TestReplicate(t *testing.T) {
	r := require.New(t)

	srcRepo, srcPath := makeEmptyPeer(t)
	dstRepo, dstPath := makeEmptyPeer(t)

	srcBS := srcRepo.BlobStore()
	//srcLog, _ := logtest.KitLogger("alice", t)
	//srcLog := log.With(log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr)), "node", "src/alice")
	srcLog := logging.Logger("alice/src")
	srcWM := blobstore.NewWantManager(srcLog, srcBS)

	dstBS := dstRepo.BlobStore()
	//dstLog, _ := logtest.KitLogger("bob", t)
	//dstLog := log.With(log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr)), "node", "dst/bob")
	dstLog := logging.Logger("bob/dst")
	dstWM := blobstore.NewWantManager(dstLog, dstBS)

	// do the dance
	pkr1, pkr2, serve := prepareConnectAndServe(t, srcRepo, dstRepo)

	pi1 := New(srcBS, srcWM)
	pi2 := New(dstBS, dstWM)

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

type testConn struct {
	io.Reader
	io.WriteCloser
	conn net.Conn

	// public keys
	local, remote []byte
}

func (conn testConn) Close() error {
	return conn.WriteCloser.Close()
}

func (conn *testConn) LocalAddr() net.Addr {
	return netwrap.WrapAddr(conn.conn.LocalAddr(), secretstream.Addr{PubKey: conn.local})
}

func (conn *testConn) RemoteAddr() net.Addr {
	return netwrap.WrapAddr(conn.conn.RemoteAddr(), secretstream.Addr{PubKey: conn.remote})
}

func (conn *testConn) SetDeadline(t time.Time) error {
	return conn.conn.SetDeadline(t)
}

func (conn *testConn) SetReadDeadline(t time.Time) error {
	return conn.conn.SetReadDeadline(t)
}

func (conn *testConn) SetWriteDeadline(t time.Time) error {
	return conn.conn.SetWriteDeadline(t)
}
