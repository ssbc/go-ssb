package tests

import (
	"context"
	"io/ioutil"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	muxtest "go.cryptoscope.co/muxrpc/test"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/sbot"
)

func TestBlobToJS(t *testing.T) {
	r := require.New(t)

	tsChan := make(chan *muxtest.Transcript, 1)

	s, _, done, cleanup := initInterop(t, `run()`,
		`sbot.blobs.want("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256",function(err, has) {
			t.true(has, "got blob")
			t.error(err, "no err")
			exit()
		})`,
		sbot.WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
			var ts muxtest.Transcript

			conn = muxtest.WrapConn(&ts, conn)
			tsChan <- &ts
			return conn, nil
		}))

	ref, err := s.BlobStore.Put(strings.NewReader("bl0000p123123"))
	r.NoError(err)
	r.Equal("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256", ref.Ref())

	defer cleanup()
	<-done

	// TODO: check wantManager for this connection is stopped when the jsbot exited

	ts := <-tsChan
	for i, dpkt := range ts.Get() {
		t.Logf("%3d: dir:%6s %v", i, dpkt.Dir, dpkt.Packet)
	}
	r.NoError(s.Close())
}

func TestBlobFromJS(t *testing.T) {
	r := require.New(t)

	const fooBarRef = "&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256"
	testRef, err := ssb.ParseBlobRef(fooBarRef) // foobar
	r.NoError(err)

	tsChan := make(chan *muxtest.Transcript, 1)

	s, _, done, cleanup := initInterop(t,
		`pull(
			pull.values([Buffer.from("foobar")]),
			sbot.blobs.add(function(err, id) {
				t.error(err, "added err")
				t.equal(id, '&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256', "blob id")
				run()
			})
		)`,
		`sbot.blobs.has(
			"&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256",
			function(err, has) {
				t.true(has, "should have blob")
				t.error(err, "has err")
				setTimeout(exit, 1500)
			})`,
		sbot.WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
			var ts muxtest.Transcript
			conn = muxtest.WrapConn(&ts, conn)
			tsChan <- &ts
			return conn, nil
		}))

	got := make(chan struct{})
	s.BlobStore.Changes().Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		defer close(got)
		notif := v.(ssb.BlobStoreNotification)
		t.Log(notif)
		r.Equal(ssb.BlobStoreOp("put"), notif.Op)
		r.Equal(fooBarRef, notif.Ref.Ref())
		return err
	}))

	err = s.WantManager.Want(testRef)
	r.NoError(err, ".Want() should not error")

	<-got

	br, err := s.BlobStore.Get(testRef)
	r.NoError(err, "should have blob")

	foobar, err := ioutil.ReadAll(br)
	r.NoError(err, "couldnt read blob")
	r.Equal("foobar", string(foobar))

	defer cleanup()
	<-done
	ts := <-tsChan
	for i, dpkt := range ts.Get() {
		t.Logf("%3d: dir:%6s %v", i, dpkt.Dir, dpkt.Packet)
	}
	r.NoError(s.Close())
}
