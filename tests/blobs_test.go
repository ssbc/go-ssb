package tests

import (
	"context"
	"fmt"
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

	s, _, done, errc, cleanup := initInterop(t, `run()`,
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
	closeErrc := make(chan error)
	go func() {
		<-done
		closeErrc <- s.Close()
		close(closeErrc)
	}()

	for err := range mergeErrorChans(errc, closeErrc) {
		r.NoError(err)
	}

	// TODO: check wantManager for this connection is stopped when the jsbot exited

	ts := <-tsChan
	for i, dpkt := range ts.Get() {
		t.Logf("%3d: dir:%6s %v", i, dpkt.Dir, dpkt.Packet)
	}
}

func TestBlobFromJS(t *testing.T) {
	r := require.New(t)

	const fooBarRef = "&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256"
	testRef, err := ssb.ParseBlobRef(fooBarRef) // foobar
	r.NoError(err)

	tsChan := make(chan *muxtest.Transcript, 1)

	s, _, done, errc, cleanup := initInterop(t,
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
		notif := v.(ssb.BlobStoreNotification)
		if ssb.BlobStoreOp("put") == notif.Op && fooBarRef == notif.Ref.Ref() {
			close(got)
		} else {
			fmt.Println("warning: wrong blob notify!", notif)
		}
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
	closeErrc := make(chan error)
	go func() {
		<-done
		closeErrc <- s.Close()
		close(closeErrc)
	}()

	for err := range mergeErrorChans(errc, closeErrc) {
		r.NoError(err)
	}

	ts := <-tsChan
	for i, dpkt := range ts.Get() {
		t.Logf("%3d: dir:%6s %v", i, dpkt.Dir, dpkt.Packet)
	}
}
