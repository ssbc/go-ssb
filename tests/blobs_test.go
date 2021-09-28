// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/v2/debug"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/internal/broadcasts"
	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestBlobToJS(t *testing.T) {
	defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	alice := ts.startJSBot(`run()`,
		`setTimeout(() => {
			sbot.blobs.want("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256",function(err, has) {
				t.true(has, "got blob")
				t.error(err, "no err")
				exit()
			})
		}, 1000)`)
	s.Replicate(alice)

	ref, err := s.BlobStore.Put(strings.NewReader("bl0000p123123"))
	r.NoError(err)
	r.Equal("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256", ref.Sigil())

	ts.wait()

	// TODO: check wantManager for this connection is stopped when the jsbot exited
}

func TestBlobFromJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	const fooBarRef = "&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256"
	testRef, err := refs.ParseBlobRef(fooBarRef) // foobar
	r.NoError(err)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	alice := ts.startJSBot(
		`
/* pinned to 1.1.14
		pull(sbot.blobs.changes(), pull.drain(function(v) {
			// migitation against blobs blocking
			// https://github.com/ssbc/ssb-blobs/pulls/17
			t.equal(v, '&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256')
			run()
		}))
*/
		pull(
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
	)
	s.Replicate(alice)

	got := make(chan struct{})
	s.BlobStore.Register(broadcasts.BlobStoreFuncEmitter(func(notif ssb.BlobStoreNotification) error {
		if ssb.BlobStoreOp("put") == notif.Op && fooBarRef == notif.Ref.Sigil() {
			close(got)
		} else {
			fmt.Println("warning: wrong blob notify!", notif)
		}
		return nil
	}))

	err = s.WantManager.Want(testRef)
	r.NoError(err, ".Want() should not error")

	<-got

	br, err := s.BlobStore.Get(testRef)
	r.NoError(err, "should have blob")

	foobar, err := ioutil.ReadAll(br)
	r.NoError(err, "couldnt read blob")
	r.Equal("foobar", string(foobar))

	ts.wait()

}

// Test sympathic blob replication with a go bot in the middle
// alice (js) creates a blob, posts about it
// go fetches the message and leaks it via a test script to claire (js2) who blobs.want()'s it then
func TestBlobWithHop(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newSession(t, nil, nil)
	// ts := newRandomSession(t)

	connI := 0
	ts.startGoBot(
		sbot.DisableEBT(true),

		sbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
			dumpPath := filepath.Join(ts.repo, "muxdump-"+strconv.Itoa(connI))
			connI++
			return debug.WrapDump(dumpPath, conn)
		}),
	)
	bob := ts.gobot

	// the JS bot leaks the blob via a message
	alice := ts.startJSBot(`
	pull(
		pull.once(Buffer.from("whopwhopwhop")),
		sbot.blobs.add(function(err, hash) {
			t.error(err)
			t.comment('got hash:'+hash)
			sbot.blobs.size(hash, (err, sz) => {
				t.error(err)
				t.comment('size'+sz)
				sbot.publish({
					type:'test',
					blob: hash,
				}, (err, msg) => {
					t.error(err)
					t.comment('leaked blob addr in:'+msg.key)
					setTimeout(run, 4000)
				})
			})
		})
	)
	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))
`, ``)
	_, err := bob.PublishLog.Publish(refs.NewContactFollow(alice))
	r.NoError(err)
	bob.Replicate(alice)
	ts.info.Log("published", "follow")

	// copy the done channel for this js instance for later
	aliceDone := ts.doneJS

	// construct a waiter for the first message
	gotMessage := make(chan struct{})
	updateSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, has := v.(int64)
		if !has {
			return fmt.Errorf("unexpected type:%T", v)
		}
		if seq == 0 {
			close(gotMessage)
		}
		return err
	})
	// register for the new message on alice's feed
	aliceLog, err := bob.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)
	done := aliceLog.Changes().Register(updateSink)

	select {
	case <-time.After(10 * time.Second):
		t.Fatal("bob did not get message from alice")

	case <-gotMessage:
		done()
		t.Log("received message from alice")
	}

	// now that bob has it, open the first message on alices feed
	msg, err := mutil.Indirect(bob.ReceiveLog, aliceLog).Get(int64(0))
	r.NoError(err)
	storedMsg, ok := msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(1, storedMsg.Seq())

	/// decode it
	type testWrap struct {
		Author  refs.FeedRef
		Content struct {
			Type string
			Blob *refs.BlobRef
		}
	}
	var m testWrap
	err = json.Unmarshal(storedMsg.ValueContentJSON(), &m)
	r.NoError(err)
	r.True(alice.Equal(m.Author), "wrong author", m.Author.ShortSigil())
	r.Equal("test", m.Content.Type, "unexpected type")
	r.NotNil(m.Content.Blob)

	// leak the reference of alices blob to claire
	wantBlob := m.Content.Blob.Sigil()
	before := fmt.Sprintf(`wantHash = %q // blob we want from alice

pull(
	sbot.blobs.changes(),
	pull.drain((evt)=> {
		t.comment('blobs:change:' + JSON.stringify(evt))
	})
)

sbot.blobs.want(wantHash, (err, has) => {
	t.error(err, 'want err?')
	exit()
})

run()
`, wantBlob)

	claire := ts.startJSBot(before, "")

	t.Logf("started claire: %s", claire.String())
	bob.Replicate(claire)

	ts.wait()
	<-aliceDone
}

func TestBlobTooBigWantedByJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	zerof, err := os.Open("/dev/zero")
	r.NoError(err)
	defer zerof.Close()

	const smallEnough = blobstore.DefaultMaxSize - 10
	small, err := s.BlobStore.Put(io.LimitReader(zerof, smallEnough))
	r.NoError(err)

	const veryLarge = blobstore.DefaultMaxSize + 10
	big, err := s.BlobStore.Put(io.LimitReader(zerof, veryLarge))
	r.NoError(err)

	t.Log("added too big", big.Sigil())
	ts.startJSBot(`timeoutLength = 600000;run()`,
		fmt.Sprintf(`
		let small = %q
		let big = %q
		setTimeout(() => {
			// this just times out, need to find a different way to handle this
			// the wants pipe returns the size and thus the jsbot doesn't request it
			sbot.blobs.want(big, (err, has) => {
				t.false(has, "did got big blob (shouldn't have)")
				t.error(err)
			})
			sbot.blobs.want(small, (err, has) => {
				t.true(has, "got small blob")
				t.error(err, "no err")
				setTimeout(exit, 5000)
			})
		}, 1000)
		`, small.Sigil(), big.Sigil()))

	ts.wait()
}

func TestBlobTooBigWantedByGo(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot(sbot.DisableEBT(true))
	s := ts.gobot

	zerof, err := os.Open("/dev/zero")
	r.NoError(err)
	defer zerof.Close()

	jsBot := ts.startJSBot(`
	
	pull(
		pull.once(Buffer.alloc(5*1024*1024-10)),
		sbot.blobs.add(function(err, smallHash) {
			t.error(err)
			t.comment('got hash:'+smallHash)

			pull(
				pull.once(Buffer.alloc(5*1024*1024+10)),
				sbot.blobs.add(function(err, bigHash) {
					t.error(err)
					t.comment('got hash:'+bigHash)
		
					sbot.publish({
						type:"test-data",
						small: smallHash,
						big: bigHash,
					}, (err, key) => {
						t.error(err)
						t.comment('published test-data')
						run()
					})
				})
			)
		})
	)
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))`, ``)
	s.Replicate(jsBot)

	jsFeedSeqs, err := s.Users.Get(storedrefs.Feed(jsBot))
	r.NoError(err)
	jsFeed := mutil.Indirect(s.ReceiveLog, jsFeedSeqs)
	tries := 10
	var testData struct {
		Type  string       `json:"test-data"`
		Small refs.BlobRef `json:"small"`
		Big   refs.BlobRef `json:"big"`
	}
	for tries > 0 {

		v, err := jsFeed.Get(int64(0))
		if err == nil {
			msg, ok := v.(refs.Message)
			r.True(ok, "not a message")

			err = json.Unmarshal(msg.ContentBytes(), &testData)
			r.NoError(err)
			break
		}
		time.Sleep(1 * time.Second)
		tries--
	}
	if tries == 0 {
		t.Fatal("did not get test-data message")
	}
	r.Equal("&SqtVEGDZDsEI53s3k9lHORXAgFjYmRK7pFDcAuYoo2c=.sha256", testData.Small.Sigil())
	r.Equal("&BcIXzoawDrJrq7ETyZxLmbL/cJzPnRCu76l5Qlgw1T4=.sha256", testData.Big.Sigil())
	s.WantManager.Want(testData.Small)
	s.WantManager.Want(testData.Big)
	time.Sleep(3 * time.Second)

	sz, err := s.BlobStore.Size(testData.Small)
	r.NoError(err)
	r.EqualValues(blobstore.DefaultMaxSize-10, sz)

	sz, err = s.BlobStore.Size(testData.Big)
	r.Error(err)
	r.EqualValues(0, sz)

	ts.wait()
}
