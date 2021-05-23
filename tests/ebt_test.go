package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2/debug"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func XTestEpidemic(t *testing.T) {
	r := require.New(t)

	// ts := newRandomSession(t)
	ts := newSession(t, nil, nil)

	// info := testutils.NewRelativeTimeLogger(nil)
	var peerCnt = 0
	ts.startGoBot(
		sbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
			fr, err := ssb.GetFeedRefFromAddr(conn.RemoteAddr())
			if err != nil {
				t.Fatal(err)
			}
			t.Log("muxwrap:", fr.Ref(), "is peer ", peerCnt)
			tpath := filepath.Join("testrun", t.Name(), fmt.Sprintf("peer-%d", peerCnt))
			peerCnt++
			return debug.WrapDump(tpath, conn)
		}),
	)
	sbot := ts.gobot

	alice, port := ts.startJSBotAsServer("alice", `

	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 13
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", i:i}))
	}
	msgs.push(mkMsg({type:"contact", "contact": testBob, "following": true}))
	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(n+1, results.length, "message count")

		sbot.identities.create((err,id) => {
			t.error(err, 'created 2nd ID')
			t.comment('the extra id:'+id)

			function mkextra(msg) {
				return function(cb) {
					setTimeout(() => {
						sbot.identities.publishAs({id, content: msg}, cb)
					},300) // wtf
				}
			}
			n = 5
			let extra = []
			for (var i = n; i>0; i--) {
				extra.push(mkextra({type:"extra", "i":i}))
			}
			require('run-series')(extra, (err, res) => {
				t.error(err)
				sbot.replicate.request(id, true)
				sbot.publish({type:"follow-test", id:id}, (err) => {
					t.error(err)
					ready()
				})
			})
		})
	})
	let done = false
	pull(
		sbot.createHistoryStream({id: testBob, live:true}),
		pull.drain((msg) => {
			t.log('message from bob:' + msg.key)
			if (msg.value.content.type == 'spam-feed') {
				const feed = msg.value.content.feed
				t.comment('got spam feed:' + feed)
				sbot.replicate.request(feed, true)
				pull(
					sbot.createHistoryStream({id: feed, live:true}),
					pull.drain((msg) => {
						t.comment('\t\talice got from spam feed:' + feed)
						if (msg.value.content.type == 'spam') {
							t.comment(JSON.tringify(msg.value.content))
							if (msg.value.content.i > 15) {
								t.ok(true, 'got enough')
								sbot.publish({type:'done'}, (err) => {
									sbot.replicate.request(feed, false)
									done=true
									setTimeout(exit, 5000)
								})
							}
						}
					})
				)
			}
		})
	)
	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => {
			t.comment(rpc.id + ": disconnected")
			t.equal(done, true, 'we are done')
		})
	})`, ``)

	sbot.Replicate(alice)

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": sbot.KeyPair.Id.Ref(),
			"name":  "test user",
		},
		map[string]interface{}{"type": "post", "text": `# hello world!`},
		map[string]interface{}{
			"type":  "about",
			"about": alice.Ref(),
			"name":  "test alice",
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := sbot.PublishLog.Publish(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}
	time.Sleep(1 * time.Second)

	wrappedAddr := netwrap.WrapAddr(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}, secretstream.Addr{PubKey: alice.PubKey()})

	connCtx, connCancel := context.WithCancel(context.TODO())
	err := sbot.Network.Connect(connCtx, wrappedAddr)
	r.NoError(err, "connect #1 failed")

	//  wait until we have all messages from alice?
	time.Sleep(5 * time.Second)

	lv, err := sbot.Users.List()
	r.NoError(err)
	r.Len(lv, 2, "just alice and bob so far")

	// load the last message from alice
	alices, err := sbot.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)

	sv, err := alices.Seq().Value()
	r.NoError(err)

	seq := sv.(margaret.BaseSeq)
	r.EqualValues(14, seq, "wrong rx seq")

	alicesMsgs := mutil.Indirect(sbot.ReceiveLog, alices)
	iv, err := alicesMsgs.Get(seq)
	r.NoError(err)
	msg := iv.(refs.Message)

	var followTest struct {
		Type string
		ID   refs.FeedRef
	}
	err = json.Unmarshal(msg.ContentBytes(), &followTest)
	r.NoError(err)

	sbot.Replicate(followTest.ID)

	// TODO: we shouldnt need this reconnect
	// but right now the ebt want-clock isn't resent on Replicate() calls alone
	connCancel()
	connCtx, connCancel = context.WithCancel(context.TODO())
	err = sbot.Network.Connect(connCtx, wrappedAddr)
	r.NoError(err, "connect #2 failed")

	// query for follow-test msgs

	/* TODO: call bob as the server
	// TODO: indicate handler if called as server or client
	claire := ts.startJSBotWithName("claire", fmt.Sprintf(``, alice.Ref()), ``)
	sbot.Replicate(claire)
	*/

	// create a pinger that stops after alice send the done message
	poop, port := ts.startJSBotAsServer("poop",
		fmt.Sprintf(`
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	let cnt = 1
	const spam = setInterval(() => {
		let msg = {
			type: 'spam',
			i: cnt
		}
		cnt++
		sbot.publish(msg, (err) => {
			t.error(err, "ping:"+cnt)
		})
	}, 1333)
	let msgs = []

	const testAlice = %q
	msgs.push(mkMsg({type:"contact", "contact": testBob, "following": true}))
	msgs.push(mkMsg({type:"contact", "contact": testAlice, "following": true}))
	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		// setTimeout(ready, 15000)
	})
	pull(
		sbot.createHistoryStream({id: testAlice, live:true}),
		pull.drain((msg) => {
			t.comment("got from alice:"+msg.value.sequence)
			if (msg.value.content.type == 'done') exit()

			if (msg.value.content.type == 'follow-test') sbot.replicate.request(msg.value.content, true)
		})
	)
	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => {
			clearInterval(spam)
			t.comment(rpc.id, "disconnected")
		})
	})`, alice.Ref()), ``)

	sbot.Replicate(poop)
	sbot.PublishLog.Publish(map[string]interface{}{
		"type": "spam-feed",
		"feed": poop.Ref(),
	})

	t.Log("published spam-feed pointer")
	time.Sleep(3 * time.Second)

	wrappedAddr = netwrap.WrapAddr(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}, secretstream.Addr{PubKey: poop.PubKey()})
	t.Log("connecting to poop")

	err = sbot.Network.Connect(context.TODO(), wrappedAddr)
	r.NoError(err, "connect #2 failed")
	time.Sleep(10 * time.Second)

	t.Log("test ended")
	connCancel()
	ts.wait()
}
