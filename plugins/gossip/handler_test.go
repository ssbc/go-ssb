package gossip

import (
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/stretchr/testify/assert"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot/plugins/test"
	"go.cryptoscope.co/secretstream"
)

func TestReplicate(t *testing.T) {
	r := assert.New(t)

	type tcase struct {
		path string
		has  margaret.BaseSeq
		pki  string
	}
	for i, tc := range []tcase{
		{"testdata/replicate1", 2, "@Z9VZfAWEFjNyo2SfuPu6dkbarqalYELwARCE4nKXyY0=.ed25519"},
		{"testdata/longTestRepo", 225, "@83JEFNo7j/kO0qrIrsCQ+h3xf7c+5Qrc0lGWJTSXrW8=.ed25519"},
	} {
		t.Log("test run", i, tc.path)

		infoAlice, _ := logtest.KitLogger("alice", t)
		infoBob, _ := logtest.KitLogger("bob", t)

		srcRepo := test.LoadTestDataPeer(t, tc.path)
		dstRepo, dstPath := test.MakeEmptyPeer(t)

		srcMlog := srcRepo.UserFeeds()
		dstMlog := dstRepo.UserFeeds()

		// check full & empty
		srcID := srcRepo.KeyPair().Id
		r.Equal(tc.pki, srcID.Ref())
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
		r.Equal(tc.has, seqVal, "wrong sequence value on testlog")

		start := time.Now()
		// do the dance
		pkr1, pkr2, serve := test.PrepareConnectAndServe(t, srcRepo, dstRepo)

		// create handlers
		h1 := &handler{Repo: srcRepo, Info: infoAlice}
		h2 := &handler{Repo: dstRepo, Info: infoBob}

		rpc1 := muxrpc.Handle(pkr1, h1)
		rpc2 := muxrpc.Handle(pkr2, h2)

		var hdone sync.WaitGroup
		hdone.Add(2)
		h1.hanlderDone = func() {
			t.Log("h1 done", time.Since(start))
			hdone.Done()
		}
		h2.hanlderDone = func() {
			t.Log("h2 done", time.Since(start))
			hdone.Done()
		}
		var finish func()
		done := make(chan struct{})
		go func() {
			hdone.Wait()
			finish()
			close(done)
		}()

		finish = serve(rpc1, rpc2)
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
		r.True(seqVal.(margaret.BaseSeq) > 0, "wrong sequence value on testlog")

		// do the dance - again.
		// should not get more messages
		// done = connectAndServe(t, srcRepo, dstRepo)
		// <-done
		// t.Log("after gossip#2")

		// dstTestLog, err = dstMlog.Get(srcMlogAddr)
		// r.NoError(err, "failed to get sublog")
		// seqVal, err = dstTestLog.Seq().Value()
		// r.NoError(err, "failed to aquire current sequence of test sublog")
		r.Equal(tc.has-1, seqVal, "wrong sequence value on testlog")

		r.NoError(srcRepo.Close(), "failed to close src repo")
		r.NoError(dstRepo.Close(), "failed to close dst repo")
		if !t.Failed() {
			os.RemoveAll(dstPath)
		}
	}
}

func BenchmarkReplicate(b *testing.B) {
	srcRepo := test.LoadTestDataPeer(b, "testdata/longTestRepo")
	bench, _ := logtest.KitLogger("bench", b)
	b.ResetTimer()

	for n := 0; n < b.N; n++ {

		dstRepo, _ := test.MakeEmptyPeer(b)

		pkr1, pkr2, serve := test.PrepareConnectAndServe(b, srcRepo, dstRepo)
		// create handlers
		h1 := &handler{Repo: srcRepo, Info: bench}
		h2 := &handler{Repo: dstRepo, Info: bench}

		rpc1 := muxrpc.Handle(pkr1, h1)
		rpc2 := muxrpc.Handle(pkr2, h2)
		serve(rpc1, rpc2)
		// dstRepo.Close()
		// os.RemoveAll(dstPath)
	}
	srcRepo.Close()
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
