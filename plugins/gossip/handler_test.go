package gossip

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"testing"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/plugins/test"
	"go.cryptoscope.co/ssb/repo"
)

func fatalOnError(err error) {
	if err != nil {
		fmt.Println("fatalOnError err:", err)
		debug.PrintStack()
		os.Exit(2)
	}
}

type testCase struct {
	path string
	has  margaret.BaseSeq
	pki  string
}

type handlerWrapper struct {
	muxrpc.Handler
	AfterConnect func()
}

func (h handlerWrapper) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	h.Handler.HandleConnect(ctx, e)
	h.AfterConnect()
}

func (tc *testCase) runTest(t *testing.T) {
	r := require.New(t)

	ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)
	defer cancel()

	var infoAlice, infoBob logging.Interface
	if testing.Verbose() {
		l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		infoAlice = log.With(l, "bot", "alice")
		infoBob = log.With(l, "bot", "bob")
	} else {
		infoAlice, _ = logtest.KitLogger("alice", t)
		infoBob, _ = logtest.KitLogger("bob", t)
	}

	srcRepo := test.LoadTestDataPeer(t, tc.path)
	srcKeyPair, err := repo.OpenKeyPair(srcRepo)
	r.NoError(err, "error opening src key pair")
	srcID := srcKeyPair.Id

	dstRepo, _ := test.MakeEmptyPeer(t)
	dstKeyPair, err := repo.OpenKeyPair(dstRepo)
	r.NoError(err, "error opening dst key pair")
	dstID := dstKeyPair.Id

	srcRootLog, err := repo.OpenLog(srcRepo)
	r.NoError(err, "error getting src root log")
	defer func() {
		err := srcRootLog.(io.Closer).Close()
		r.NoError(err, "error closing src root log")
	}()

	srcMlog, _, srcMlogServe, err := multilogs.OpenUserFeeds(srcRepo)
	r.NoError(err, "error getting src userfeeds multilog")
	defer func() {
		err := srcMlog.Close()
		r.NoError(err, "error closing src user feeds multilog")
	}()

	go func() {
		err := srcMlogServe(ctx, srcRootLog, true)
		fatalOnError(errors.Wrap(err, "error serving src user feeds multilog"))
	}()

	srcGraphBuilder, srcGraphBuilderServe, err := indexes.OpenContacts(infoAlice, srcRepo)
	r.NoError(err, "error getting src contacts index")
	defer func() {
		err := srcGraphBuilder.Close()
		r.NoError(err, "error closing src graph builder")
	}()

	go func() {
		err := srcGraphBuilderServe(ctx, srcRootLog, true)
		fatalOnError(errors.Wrap(err, "error serving src contacts index"))
	}()

	dstRootLog, err := repo.OpenLog(dstRepo)
	r.NoError(err, "error getting dst root log")
	defer func() {
		err := dstRootLog.(io.Closer).Close()
		r.NoError(err, "error closing dst root log")
	}()

	dstMlog, _, dstMlogServe, err := multilogs.OpenUserFeeds(dstRepo)
	r.NoError(err, "error getting dst userfeeds multilog")
	defer func() {
		err := dstMlog.Close()
		r.NoError(err, "error closing dst userfeeds multilog")
	}()

	go func() {
		err := dstMlogServe(ctx, dstRootLog, true)
		fatalOnError(errors.Wrap(err, "error serving dst user feeds multilog"))
	}()

	dstGraphBuilder, dstGraphBuilderServe, err := indexes.OpenContacts(infoAlice, dstRepo)
	r.NoError(err, "error getting dst contacts index")
	defer func() {
		err := dstGraphBuilder.Close()
		r.NoError(err, "error closing dst graph builder")
	}()

	go func() {
		err := dstGraphBuilderServe(ctx, dstRootLog, true)
		fatalOnError(errors.Wrap(err, "error serving dst contacts index"))
	}()

	// check full & empty
	r.Equal(tc.pki, srcID.Ref())
	srcMlogAddr := srcID.StoredAddr()
	// XXX: tests are broken because the repo data that is commited was created for the old index format
	// TODO: make tool to generate these or rewrite this test
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
	pkr1, pkr2, _, serve := test.PrepareConnectAndServe(t, srcRepo, dstRepo)

	var hdone sync.WaitGroup
	hdone.Add(2)

	// create handlers
	h1 := handlerWrapper{
		Handler: &handler{
			promisc:      true,
			Id:           srcID,
			RootLog:      srcRootLog,
			UserFeeds:    srcMlog,
			GraphBuilder: srcGraphBuilder,
			Info:         infoAlice,
		},
		AfterConnect: func() {
			infoAlice.Log("event", "handler done", "time-since", time.Since(start))
			hdone.Done()
		},
	}

	h2 := handlerWrapper{
		Handler: &handler{
			promisc:      true,
			Id:           dstID,
			RootLog:      dstRootLog,
			UserFeeds:    dstMlog,
			GraphBuilder: dstGraphBuilder,
			Info:         infoBob,
		},
		AfterConnect: func() {
			infoBob.Log("event", "handler done", "time-since", time.Since(start))
			hdone.Done()
		},
	}

	rpc1 := muxrpc.Handle(pkr1, h1)
	rpc2 := muxrpc.Handle(pkr2, h2)

	finish := make(chan func())
	done := make(chan struct{})
	go func() {
		hdone.Wait()
		(<-finish)()
		close(done)
	}()

	finish <- serve(rpc1, rpc2)
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
	r.Equal(tc.has, seqVal, "wrong sequence value on testlog")

}

func TestReplicate(t *testing.T) {
	for _, tc := range []testCase{
		{"testdata/replicate1", 2, "@Z9VZfAWEFjNyo2SfuPu6dkbarqalYELwARCE4nKXyY0=.ed25519"},
		{"testdata/largeRepo", 431, "@qhSpPqhWyJBZ0/w+ERa6WZvRWjaXu0dlep6L+Xi6PQ0=.ed25519"},
	} {
		t.Run(tc.path, tc.runTest)
	}
}

func BenchmarkReplicate(b *testing.B) {
	r := require.New(b)
	var wg sync.WaitGroup

	srcRepo := test.LoadTestDataPeer(b, "testdata/largeRepo")
	bench, _ := logtest.KitLogger("bench", b)
	b.ResetTimer()

	srcRootLog, err := repo.OpenLog(srcRepo)
	r.NoError(err)

	srcMlog, _, srcMlogServe, err := multilogs.OpenUserFeeds(srcRepo)
	r.NoError(err)

	wg.Add(1)
	go func() {
		err := srcMlogServe(context.TODO(), srcRootLog, true)
		fatalOnError(errors.Wrap(err, "srcMlogServe error"))
		wg.Done()
	}()

	srcKeyPair, _ := repo.OpenKeyPair(srcRepo)
	srcID := srcKeyPair.Id
	srcGraphBuilder, srcGraphBuilderServe, err := indexes.OpenContacts(bench, srcRepo)
	r.NoError(err)
	wg.Add(1)
	go func() {
		err := srcGraphBuilderServe(context.TODO(), srcRootLog, true)
		fatalOnError(errors.Wrap(err, "srcGraphBuilderServe error"))
		wg.Done()
	}()

	for n := 0; n < b.N; n++ {

		dstRepo, _ := test.MakeEmptyPeer(b)
		r.NoError(err)
		dstRootLog, _ := repo.OpenLog(dstRepo)
		r.NoError(err)
		dstMlog, _, dstMlogServe, _ := multilogs.OpenUserFeeds(dstRepo)
		r.NoError(err)

		wg.Add(1)
		go func() {
			err := dstMlogServe(context.TODO(), dstRootLog, true)
			b.Log("dstMlogServe error:", err)
			wg.Done()
		}()
		dstKeyPair, _ := repo.OpenKeyPair(dstRepo)
		dstID := dstKeyPair.Id
		dstGraphBuilder, dstGraphBuilderServe, _ := indexes.OpenContacts(bench, dstRepo)

		wg.Add(1)
		go func() {
			err := dstGraphBuilderServe(context.TODO(), dstRootLog, true)
			b.Log("dstGraphBuilderServe error:", err)
			wg.Done()
		}()

		pkr1, pkr2, _, serve := test.PrepareConnectAndServe(b, srcRepo, dstRepo)
		// create handlers
		h1 := &handler{
			Id:           srcID,
			RootLog:      srcRootLog,
			UserFeeds:    srcMlog,
			GraphBuilder: srcGraphBuilder,
			Info:         bench,
		}
		h2 := &handler{
			Id:           dstID,
			RootLog:      dstRootLog,
			UserFeeds:    dstMlog,
			GraphBuilder: dstGraphBuilder,
			Info:         bench,
		}

		rpc1 := muxrpc.Handle(pkr1, h1)
		rpc2 := muxrpc.Handle(pkr2, h2)
		serve(rpc1, rpc2)
		// dstRepo.Close()
		// os.RemoveAll(dstPath)
	}
	wg.Wait()
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
