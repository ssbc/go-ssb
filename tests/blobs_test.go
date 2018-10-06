package tests

import (
	"io/ioutil"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	muxtest "go.cryptoscope.co/muxrpc/test"
	ssb "go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/sbot"
)

func TestBlobToJS(t *testing.T) {
	r := require.New(t)

	tsChan := make(chan *muxtest.Transcript, 1)

	s, _, exited := initInterop(t, `run()`,
		`sbot.blobs.want("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256",function(err, has) {
			t.true(has, "got blob")
			t.end(err, "no err")
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

	<-exited
	ts := <-tsChan
	for i, dpkt := range ts.Get() {
		t.Logf("%3d: dir:%6s %v", i, dpkt.Dir, dpkt.Packet)
	}
}

func TestBlobFromJS(t *testing.T) {
	r := require.New(t)

	testRef, err := ssb.ParseBlobRef("&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256") // foobar
	r.NoError(err)

	tsChan := make(chan *muxtest.Transcript, 1)

	s, _, exited := initInterop(t,
		`pull(
			pull.values([Buffer.from("foobar")]),
			sbot.blobs.add(function(err, id) {
				t.error(err, "added")
				t.equal(id, '&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256', "blob id")
				run()
			})
		)`,
		`sbot.blobs.has(
			"&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256",
			function(err, has) {
				t.true(has, "should have blob")
				t.end(err)
				setTimeout(exit, 3000)
			})`,
		sbot.WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
			var ts muxtest.Transcript

			conn = muxtest.WrapConn(&ts, conn)
			tsChan <- &ts
			return conn, nil
		}))

	err = s.WantManager.Want(testRef)
	r.NoError(err, ".Want() should not error")

	time.Sleep(5 * time.Second)

	br, err := s.BlobStore.Get(testRef)
	r.NoError(err, "should have blob")

	foobar, err := ioutil.ReadAll(br)
	r.NoError(err, "couldnt read blob")
	r.Equal("foobar", string(foobar))

	<-exited
	ts := <-tsChan
	for i, dpkt := range ts.Get() {
		t.Logf("%3d: dir:%6s %v", i, dpkt.Dir, dpkt.Packet)
	}
}
