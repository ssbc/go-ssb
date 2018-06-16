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
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot/repo"
	"go.cryptoscope.co/secretstream"
)

func TestReplicate(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// persisted test data
	srcRepo, err := repo.New("testdata/replicate1")
	r.NoError(err, "failed to create srcRepo")
	srcKp := srcRepo.KeyPair()
	r.NotNil(srcKp)

	// new target repo
	dstPath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)
	dstRepo, err := repo.New(dstPath)
	r.NoError(err, "failed to create dstRepo")
	dstKp := dstRepo.KeyPair()
	r.NotNil(dstKp)

	// check full & empty
	srcKf, err := srcRepo.KnownFeeds()
	r.NoError(err, "failed to get known feeds from source")
	r.Len(srcKf, 1)
	r.Equal(margaret.Seq(10), srcKf[srcKp.Id.Ref()])

	dstKf, err := dstRepo.KnownFeeds()
	r.NoError(err, "failed to get known feeds from source")
	r.Len(dstKf, 0)

	// construct transfer
	p1, p2 := net.Pipe()
	infologSrc, _ := logtest.KitLogger("srcRepo", t)
	tc1 := testConn{
		Reader:      p1,
		WriteCloser: p1,
		conn:        p1,
		local:       srcKp.Pair.Public[:],
		remote:      dstKp.Pair.Public[:],
	}
	pkr1 := muxrpc.NewPacker(tc1) //codec.Wrap(infologSrc, tc1))

	infologDst, _ := logtest.KitLogger("dstRepo", t)
	tc2 := testConn{
		Reader:      p2,
		WriteCloser: p2,
		conn:        p2,
		local:       dstKp.Pair.Public[:],
		remote:      srcKp.Pair.Public[:],
	}
	pkr2 := muxrpc.NewPacker(tc2) //codec.Wrap(infologDst, tc2))

	// create handlers
	h1 := Handler{Repo: srcRepo, Info: infologSrc}
	h2 := Handler{Repo: dstRepo, Info: infologDst}

	a.Equal(tc1.LocalAddr().String(), tc2.RemoteAddr().String())
	a.Equal(tc2.LocalAddr().String(), tc1.RemoteAddr().String())

	// serve
	rpc1 := muxrpc.HandleWithRemote(pkr1, &h1, tc2.LocalAddr())
	rpc2 := muxrpc.HandleWithRemote(pkr2, &h2, tc1.LocalAddr())

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
	time.Sleep(3 * time.Second)
	r.NoError(rpc1.Terminate())
	r.NoError(rpc2.Terminate())

	// check data ended up on the target
	afterkf, err := dstRepo.KnownFeeds()
	r.NoError(err)
	r.Len(afterkf, 1)
	r.Equal(margaret.Seq(10), afterkf[srcKp.Id.Ref()])

	seqs, err := dstRepo.FeedSeqs(srcKp.Id)
	r.NoError(err)
	r.Len(seqs, 10)

	wg.Wait()

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
