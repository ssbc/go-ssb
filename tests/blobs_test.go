// SPDX-License-Identifier: MIT

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

func TestBlobToJS(t *testing.T) {
	r := require.New(t)


	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	ts.startJSBot(`run()`,
		`setTimeout(() => {
			sbot.blobs.want("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256",function(err, has) {
				t.true(has, "got blob")
				t.error(err, "no err")
				exit()
			})
		}, 1000)`)

	ref, err := s.BlobStore.Put(strings.NewReader("bl0000p123123"))
	r.NoError(err)
	r.Equal("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256", ref.Ref())

	ts.wait()

	// TODO: check wantManager for this connection is stopped when the jsbot exited

}

func TestBlobFromJS(t *testing.T) {
	r := require.New(t)

	const fooBarRef = "&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256"
	testRef, err := ssb.ParseBlobRef(fooBarRef) // foobar
	r.NoError(err)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	ts.startJSBot(
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

	ts.wait()

}

func TestBlobWithHop(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)

	ts.startGoBot()
	bob := ts.gobot

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
					run()
				})
			})
		})
	  )
	
	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))
`, ``)

	aliceDone := ts.doneJS
	newSeq, err := bob.PublishLog.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   alice.Ref(),
		"following": true,
	})
	r.NoError(err, "failed to publish contact message")
	r.NotNil(newSeq)

	time.Sleep(2 * time.Second)

	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err := uf.Get(alice.StoredAddr())
	r.NoError(err)
	seq, err := aliceLog.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(0), seq)

	var wantBlob *ssb.BlobRef

	seqMsg, err := aliceLog.Get(margaret.BaseSeq(0))
	r.NoError(err)
	r.Equal(seqMsg, margaret.BaseSeq(1))

	msg, err := bob.RootLog.Get(seqMsg.(margaret.BaseSeq))
	r.NoError(err)
	storedMsg, ok := msg.(ssb.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.Equal(storedMsg.Seq(), margaret.BaseSeq(1).Seq())

	type testWrap struct {
		Author  ssb.FeedRef
		Content struct {
			Type string
			Blob *ssb.BlobRef
		}
	}
	var m testWrap
	err = json.Unmarshal(storedMsg.ValueContentJSON(), &m)
	r.NoError(err)
	r.True(alice.Equal(&m.Author), "wrong author")
	r.Equal("test", m.Content.Type)
	r.NotNil(m.Content.Blob)

	wantBlob = m.Content.Blob

	// bob.WantManager.Want(wantBlob)

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
`, wantBlob.Ref())

	claire := ts.startJSBot(before, "")

	t.Logf("started claire: %s", claire.Ref())
	newSeq, err = bob.PublishLog.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   claire.Ref(),
		"following": true,
	})
	r.NoError(err, "failed to publish 2nd contact message")
	r.NotNil(newSeq)

	ts.wait()
	<-aliceDone
}
