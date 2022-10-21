// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package tests

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ssbc/go-ssb/internal/leakcheck"
	"github.com/ssbc/go-ssb/internal/mutil"
	"github.com/ssbc/go-ssb/internal/storedrefs"
	"github.com/ssbc/go-ssb/internal/testutils"
	"github.com/ssbc/go-ssb/sbot"
	refs "github.com/ssbc/go-ssb-refs"
)

func TestFeedFromJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	const n = 23

	ts := newRandomSession(t)

	ts.startGoBot()
	bob := ts.gobot

	// just for keygen, needed later
	claire := ts.startJSBotWithName("claire", `exit()`, "")

	alice := ts.startJSBotWithName("alice", fmt.Sprintf(`
	let claireRef = %q
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 23
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", i:i}))
	}
	msgs.push(mkMsg({type: 'contact', contact: claireRef, following: true}))

	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))

	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(n+1, results.length, "message count")
		run() // triggers connect and after block
	})
`, claire.String()), ``)

	bob.Replicate(alice)

	<-ts.doneJS

	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err := uf.Get(storedrefs.Feed(alice))
	r.NoError(err)

	r.EqualValues(n, aliceLog.Seq())

	var lastMsg string
	for i := 0; i < n; i++ { // don't check the contact:following message from A to C
		msg, err := mutil.Indirect(bob.ReceiveLog, aliceLog).Get(int64(i))
		r.NoError(err)
		storedMsg, ok := msg.(refs.Message)
		r.True(ok, "wrong type of message: %T", msg)
		r.EqualValues(i+1, storedMsg.Seq())

		type testWrap struct {
			Author  refs.FeedRef
			Content struct {
				Type, Text string
				I          int
			}
		}
		var m testWrap
		err = json.Unmarshal(storedMsg.ValueContentJSON(), &m)
		r.NoError(err)
		r.True(alice.Equal(m.Author), "wrong author")
		r.Equal(m.Content.Type, "test")
		r.Equal(m.Content.Text, "foo")
		r.Equal(m.Content.I, n-i, "wrong I on msg: %d", i)
		if i == n-1 {
			lastMsg = storedMsg.Key().String()
		}
	}

	before := fmt.Sprintf(`
aliceRef = %q // global - pubKey of the first alice

t.comment('shouldnt have alices feed:' + aliceRef)

sbot.on('rpc:connect', (rpc) => {
  rpc.on('closed', () => { 
    t.comment('now should have feed:' + aliceRef)
    pull(
      sbot.createUserStream({id:aliceRef, reverse:true, limit: 2}),
      pull.collect(function(err, msgs) {
        t.error(err, 'query worked')
		t.equal(2, msgs.length, 'got all the messages')
		// skip the contact message
		t.equal('contact', msgs[0].value.content.type, 'latest sequence')
        t.equal(%q, msgs[1].key, 'latest keys match')
        t.equal(23, msgs[1].value.sequence, 'latest sequence')
        exit()
      })
    )
  })
})

sbot.publish({type: 'contact', contact: aliceRef, following: true}, function(err, msg) {
  t.error(err, 'follow:' + aliceRef)

sbot.friends.get({src: alice.id, dest: aliceRef}, function(err, val) {
  t.error(err, 'friends.get of new contact')
  t.equals(val[alice.id], true, 'is following')

pull(
  sbot.createUserStream({id:aliceRef}),
  pull.collect(function(err, vals){
    t.error(err)
    t.equal(0, vals.length)
    run() // connect to go-sbot
  })
)

}) // friends.get

}) // publish`, alice.String(), lastMsg)
	claire = ts.startJSBotWithName("claire", before, "")

	t.Logf("started claire: %s", claire.String())
	bob.Replicate(claire)

	ts.wait()
}

func TestFeedFromGoNotLive(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot(sbot.DisableEBT(true))
	s := ts.gobot

	before := `fromKey = testBob
	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => {
			t.comment('now should have feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey, reverse:true, limit: 3}),
				pull.collect(function(err, msgs) {
					t.error(err, 'query worked')
					t.equal(msgs.length, 3, 'got all the messages')
					// t.comment(JSON.stringify(msgs[0]))
					t.equal(msgs[0].value.sequence, 3, 'sequence:0')
					t.equal(msgs[1].value.sequence, 2, 'sequence:1')
					t.equal(msgs[2].value.sequence, 1, 'sequence:2')
					exit()
				})
			)
		})
	})

	// sbot.publish({type: 'test', test: true}, (err, msg)  => {
	// 	t.error(err, 'test:' + msg.sequence )

	sbot.publish({type: 'contact', contact: fromKey, following: true}, (err, msg)  => {
		t.error(err, 'follow:' + fromKey)

		sbot.friends.get({src: alice.id, dest: fromKey}, (err, val)  => {
			t.error(err, 'friends.get of new contact')
			t.equals(val[alice.id], true, 'is following')

			t.comment('shouldnt have bobs feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey}),
				pull.collect((err, vals) => {
					t.error(err)
					t.equal(0, vals.length)
					sbot.publish({type: 'about', about: fromKey, name: 'test bob'}, (err, msg)  => {
						t.error(err, 'about:' + msg.key)
						setTimeout(run, 1000)
					})
				})
			)

}) // friends.get

}) // publish contact

// }) // publish test
`

	alice := ts.startJSBot(before, "")
	s.Replicate(alice)

	var tmsgs = []interface{}{
		refs.NewAboutName(s.KeyPair.ID(), "test bot"),
		refs.NewPost("# hello world!"),
		refs.NewAboutName(alice, "test alice"),
	}
	for i, msg := range tmsgs {
		newSeq, err := s.PublishLog.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	<-ts.doneJS

	aliceIdx, err := s.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)
	r.EqualValues(2-1, aliceIdx.Seq(), "expected two messages on alice's feed (0 indexed)")

	// bmap, err := s.Users.LoadInternalBitmap(storedrefs.Feed(alice))
	// r.NoError(err)
	// t.Log(bmap.ToArray())

	aliceMsgs := mutil.Indirect(s.ReceiveLog, aliceIdx)

	msg, err := aliceMsgs.Get(0)
	r.NoError(err)
	storedMsg, ok := msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(storedMsg.Seq(), 1, "expected first message")

	s.Network.GetConnTracker().CloseAll()
	ts.wait()

	s.Shutdown()
	s.Close()

	t.Log("restarting for integrity check")
	ts.startGoBot()

	s = ts.gobot
	err = s.FSCK(sbot.FSCKWithMode(sbot.FSCKModeSequences))
	r.NoError(err)

	aliceMsgs = mutil.Indirect(s.ReceiveLog, aliceIdx)

	msg, err = aliceMsgs.Get(aliceMsgs.Seq())
	r.NoError(err)
	storedMsg, ok = msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(2, storedMsg.Seq(), "expected last message")

	bobIndex, err := s.Users.Get(storedrefs.Feed(s.KeyPair.ID()))
	r.NoError(err)
	bobMsgs := mutil.Indirect(s.ReceiveLog, bobIndex)

	bmap, err := s.Users.LoadInternalBitmap(storedrefs.Feed(s.KeyPair.ID()))
	r.NoError(err)
	t.Log("bots msgs:", bmap.ToArray())

	t.Log("just bob")
	testutils.StreamLog(t, bobMsgs)

	t.Log("all")
	testutils.StreamLog(t, s.ReceiveLog)

	r.EqualValues(3-1, bobMsgs.Seq(), "bob should have 3 message (0 indexed)")

	msg, err = bobMsgs.Get(2)
	r.NoError(err)
	storedMsg, ok = msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(3, storedMsg.Seq(), "expected msg 3 from bob")

	err = s.FSCK(sbot.FSCKWithMode(sbot.FSCKModeSequences))
	r.NoError(err)

	ts.wait()
}

func TestFeedFromGoLive(t *testing.T) {
	defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)

	ts.startGoBot(
		sbot.WithPromisc(true),
		sbot.DisableEBT(true),
	)
	s := ts.gobot

	before := `fromKey = testBob
	pull(
		sbot.createHistoryStream({id:fromKey, live:true}),
		pull.drain(function(msg) {
			t.comment("got message!"+ msg.value.sequence)
			// t.comment(JSON.stringify(msg).key)
			if (msg.value.sequence == 5) {
				
				t.comment("waited")
				exit()
			}
		})
	)
	sbot.publish({type: 'contact', contact: fromKey, following: true}, function(err, msg) {
		t.error(err, 'follow:' + fromKey)

		sbot.friends.get({src: alice.id, dest: fromKey}, function(err, val) {
			t.error(err, 'friends.get of new contact')
			t.equals(val[alice.id], true, 'is following')

			t.comment('shouldnt have bobs feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey}),
				pull.collect(function(err, vals){
					t.error(err)
					t.equal(0, vals.length)
					sbot.publish({type: 'about', about: fromKey, name: 'test bob'}, function(err, msg) {
						t.error(err, 'about:' + msg.key)
						setTimeout(run, 1000) // give go bot a moment to publish
					})
				})
			)

}) // friends.get

}) // publish`

	alice := ts.startJSBot(before, "")

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": s.KeyPair.ID().String(),
			"name":  "test user",
		},
		map[string]interface{}{
			"type":      "contact",
			"contact":   alice.String(),
			"following": true,
		},
		map[string]interface{}{
			"type": "text",
			"text": `# hello world!`,
		},
		map[string]interface{}{
			"type":  "about",
			"about": alice.String(),
			"name":  "test alice",
		},
	}
	for i, msg := range tmsgs {
		ref, err := s.PublishLog.Publish(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotZero(ref)
	}

	time.Sleep(2 * time.Second)
	ref, err := s.PublishLog.Publish(map[string]interface{}{
		"type": "test",
		"live": true,
	})
	r.NoError(err)
	r.NotNil(ref)
	fmt.Println("send late msg")

	<-ts.doneJS

	aliceLog, err := s.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)

	seqMsg, err := aliceLog.Get(1)
	r.NoError(err)
	msg, err := s.ReceiveLog.Get(seqMsg.(int64))
	r.NoError(err)
	storedMsg, ok := msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(2, storedMsg.Seq())

	ts.wait()
}
