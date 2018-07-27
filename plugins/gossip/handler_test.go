package gossip

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/repo"
	"go.cryptoscope.co/secretstream"
)

func loadTestDataPeer(t *testing.T, repopath string) sbot.Repo {
	r := require.New(t)
	repo, err := repo.New(repopath)
	r.NoError(err, "failed to load testData repo")
	r.NotNil(repo.KeyPair())
	return repo
}

func makeEmptyPeer(t *testing.T) (sbot.Repo, string) {
	r := require.New(t)
	dstPath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)
	dstRepo, err := repo.New(dstPath)
	r.NoError(err, "failed to create emptyRepo")
	r.NotNil(dstRepo.KeyPair())
	return dstRepo, dstPath
}

func connectAndServe(t *testing.T, alice, bob sbot.Repo, tout time.Duration) <-chan struct{} {
	r := require.New(t)
	keyAlice := alice.KeyPair()
	keyBob := bob.KeyPair()

	p1, p2 := net.Pipe()
	infoAlice, _ := logtest.KitLogger("alice", t)
	infoBob, _ := logtest.KitLogger("bob", t)
	tc1 := testConn{
		Reader: p1, WriteCloser: p1, conn: p1,
		local:  keyAlice.Pair.Public[:],
		remote: keyBob.Pair.Public[:],
	}
	tc2 := testConn{
		Reader: p2, WriteCloser: p2, conn: p2,
		local:  keyBob.Pair.Public[:],
		remote: keyAlice.Pair.Public[:],
	}
	var rwc1, rwc2 io.ReadWriteCloser = tc1, tc2
	/* logs every muxrpc packet
	if testing.Verbose() {
		rwc1 = codec.Wrap(infoAlice, rwc1)
		rwc2 = codec.Wrap(infoBob, rwc2)
	}
	*/
	pkr1, pkr2 := muxrpc.NewPacker(rwc1), muxrpc.NewPacker(rwc2)

	// create handlers
	h1 := Handler{Repo: alice, Info: infoAlice}
	h2 := Handler{Repo: bob, Info: infoBob}

	// serve
	rpc1 := muxrpc.HandleWithRemote(pkr1, &h1, tc1.RemoteAddr())
	rpc2 := muxrpc.HandleWithRemote(pkr2, &h2, tc2.RemoteAddr())

	ctx := context.Background()
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

	// wait TODO: close handling
	done := make(chan struct{})
	go func() {
		time.Sleep(tout)
		r.NoError(rpc1.Terminate())
		r.NoError(rpc2.Terminate())
		wg.Wait()
		close(done)
	}()

	return done
}

func TestReplicate(t *testing.T) {
	r := assert.New(t)

	srcRepo := loadTestDataPeer(t, "testdata/replicate1")
	dstRepo, dstPath := makeEmptyPeer(t)

	srcMlog := srcRepo.UserFeeds()
	dstMlog := dstRepo.UserFeeds()

	// check full & empty
	srcID := srcRepo.KeyPair().Id
	srcMlogAddr := librarian.Addr(srcID.ID)
	has, err := multilog.Has(srcMlog, srcMlogAddr)
	r.NoError(err)
	r.True(has, "source should have the testLog")
	has, err = multilog.Has(dstMlog, srcMlogAddr)
	r.NoError(err)
	r.False(has, "destination should not have the testLog already")

	testLog, err := srcMlog.Get(srcMlogAddr)
	r.NoError(err, "failed to get sublog")
	seqVal, err := testLog.Seq().Value()
	r.NoError(err, "failed to aquire current sequence of test sublog")
	r.Equal(margaret.BaseSeq(2), seqVal, "wrong sequence value on testlog")

	// do the dance
	done := connectAndServe(t, srcRepo, dstRepo, 3*time.Second)
	<-done

	t.Log("after gossip")

	// check data ended up on the target
	has, err = multilog.Has(dstMlog, srcMlogAddr)
	r.NoError(err)
	r.True(has, "destination should now have the testLog already")

	dstTestLog, err := dstMlog.Get(srcMlogAddr)
	r.NoError(err, "failed to get sublog")
	seqVal, err = dstTestLog.Seq().Value()
	r.NoError(err, "failed to aquire current sequence of test sublog")
	r.Equal(margaret.BaseSeq(2), seqVal, "wrong sequence value on testlog")

	if !t.Failed() {
		os.RemoveAll(dstPath)
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
