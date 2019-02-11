package test

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

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/debug"
	muxtest "go.cryptoscope.co/muxrpc/test"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"

	"go.cryptoscope.co/ssb/repo"
)

func LoadTestDataPeer(t testing.TB, repopath string) repo.Interface {
	r := require.New(t)
	rp := repo.New(repopath)

	kp, err := repo.OpenKeyPair(rp)
	r.NoError(err, "error opening keypair")
	r.NotNil(kp, "key pair is nil")
	return rp
}

func MakeEmptyPeer(t testing.TB) (repo.Interface, string) {
	r := require.New(t)
	dstPath, err := ioutil.TempDir("", strings.Replace(t.Name(), "/", "_", -1))
	r.NoError(err)
	dstRepo := repo.New(dstPath)
	return dstRepo, dstPath
}

func PrepareConnectAndServe(t testing.TB, alice, bob repo.Interface) (muxrpc.Packer, muxrpc.Packer, *muxtest.Transcript, func(rpc1, rpc2 muxrpc.Endpoint) func()) {
	r := require.New(t)
	keyAlice, err := repo.OpenKeyPair(alice)
	r.NoError(err, "error opening alice's key pair")

	keyBob, err := repo.OpenKeyPair(bob)
	r.NoError(err, "error opening bob's key pair")

	p1, p2 := net.Pipe()

	var infoAlice, infoBob logging.Interface
	if testing.Verbose() {
		l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		infoAlice = log.With(l, "testsrc", "alice")
		infoBob = log.With(l, "testdst", "bob")
	} else {
		infoAlice, _ = logtest.KitLogger("alice", t)
		infoBob, _ = logtest.KitLogger("bob", t)
	}

	tc1 := &TestConn{
		Reader: p1, WriteCloser: p1, conn: p1,
		local:  keyAlice.Pair.Public[:],
		remote: keyBob.Pair.Public[:],
	}
	tc2 := &TestConn{
		Reader: p2, WriteCloser: p2, conn: p2,
		local:  keyBob.Pair.Public[:],
		remote: keyAlice.Pair.Public[:],
	}

	var conn1, conn2 net.Conn = tc1, tc2

	// logs every muxrpc packet
	if testing.Verbose() {
		conn1 = debug.WrapConn(infoAlice, conn1)
		conn2 = debug.WrapConn(infoBob, conn2)
	}

	var ts muxtest.Transcript

	//conn1 = muxtest.WrapConn(&ts, conn1)

	return muxrpc.NewPacker(conn1), muxrpc.NewPacker(conn2), &ts, func(rpc1, rpc2 muxrpc.Endpoint) func() {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)

		var (
			wg         sync.WaitGroup
			err1, err2 error
		)

		wg.Add(2)
		go func() {
			err1 = rpc1.(muxrpc.Server).Serve(ctx)
			wg.Done()
		}()

		go func() {
			err2 = rpc2.(muxrpc.Server).Serve(ctx)
			wg.Done()
		}()

		return func() {
			cancel()

			r.NoError(rpc1.Terminate())
			r.NoError(rpc2.Terminate())

			wg.Wait()
			r.NoError(err1, "rpc1 serve err")
			r.NoError(err2, "rpc2 serve err")
		}
	}
}

type TestConn struct {
	io.Reader
	io.WriteCloser
	conn net.Conn

	// public keys
	local, remote []byte
}

func (conn TestConn) Close() error {
	return conn.WriteCloser.Close()
}

func (conn *TestConn) LocalAddr() net.Addr {
	return netwrap.WrapAddr(conn.conn.LocalAddr(), secretstream.Addr{PubKey: conn.local})
}

func (conn *TestConn) RemoteAddr() net.Addr {
	return netwrap.WrapAddr(conn.conn.RemoteAddr(), secretstream.Addr{PubKey: conn.remote})
}

func (conn *TestConn) SetDeadline(t time.Time) error {
	return conn.conn.SetDeadline(t)
}

func (conn *TestConn) SetReadDeadline(t time.Time) error {
	return conn.conn.SetReadDeadline(t)
}

func (conn *TestConn) SetWriteDeadline(t time.Time) error {
	return conn.conn.SetWriteDeadline(t)
}
