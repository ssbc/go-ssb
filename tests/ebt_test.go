package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	refs "go.mindeco.de/ssb-refs"
)

func TestEpidemic(t *testing.T) {
	r := require.New(t)

	// ts := newRandomSession(t)
	ts := newSession(t, nil, nil)

	// info := testutils.NewRelativeTimeLogger(nil)
	ts.startGoBot(
	// TODO: the close handling on the debug wrapper is bugged, using it stalls the tests at the end
	// sbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
	// 	fr, err := ssb.GetFeedRefFromAddr(conn.RemoteAddr())
	// 	return debug.WrapConn(log.With(info, "remote", fr.ShortRef()), conn), err
	// }),
	)
	sbot := ts.gobot

	// just for keygen, needed later
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
				extra.push(mkextra({type:"extra", i:i}))
			}
			require('run-series')(extra, (err, res) => {
				t.error(err)
				// t.comment(JSON.stringify(res))
				sbot.replicate.request(id, true)
				sbot.publish({type:"follow-test", id:id}, (err) => {
					t.error(err)
					ready() // triggers connect and after block
				})
			})
		})
	})
	let done = false
	pull(
		sbot.createHistoryStream({id: testBob, live:true}),
		pull.drain((msg) => {
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
		refs.NewContactFollow(alice),
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
	}, secretstream.Addr{PubKey: alice.ID})

	err := sbot.Network.Connect(context.TODO(), wrappedAddr)
	r.NoError(err, "connect #1 failed")

	//  wait until we have all messages from alice?
	time.Sleep(5 * time.Second)

	alices, err := sbot.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)

	sv, err := alices.Seq().Value()
	r.NoError(err)

	lv, err := sbot.Users.List()
	r.NoError(err)
	r.Len(lv, 2)

	seq := sv.(margaret.BaseSeq)
	r.EqualValues(14, seq)
	alicesMsgs := mutil.Indirect(sbot.ReceiveLog, alices)
	iv, err := alicesMsgs.Get(seq)
	r.NoError(err)
	msg := iv.(refs.Message)

	var followTest struct{ ID *refs.FeedRef }
	err = json.Unmarshal(msg.ContentBytes(), &followTest)
	r.NoError(err)

	sbot.Replicate(followTest.ID)

	// query for follow-test msgs

	/* TODO: call bob as the server
	// TODO: indicate handler if called as server or client
	claire := ts.startJSBotWithName("claire", fmt.Sprintf(``, alice.Ref()), ``)
	sbot.Replicate(claire)
	*/
	t.Log("plz stayyyyyy")
	time.Sleep(45 * time.Second)
	t.Log("donnneee")
	return

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
			t.comment(rpc.id, "disconnected")
		})
	})`, alice.Ref()), ``)

	sbot.Replicate(poop)
	sbot.PublishLog.Publish(map[string]interface{}{
		"type": "spam-feed",
		"feed": poop.Ref(),
	})

	t.Log("hmmm")
	time.Sleep(3 * time.Second)
	wrappedAddr = netwrap.WrapAddr(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}, secretstream.Addr{PubKey: poop.ID})
	t.Log("connecting to poop")
	err = sbot.Network.Connect(context.TODO(), wrappedAddr)
	r.NoError(err, "connect #2 failed")
	time.Sleep(10 * time.Second)
	t.Log("test ended")
	ts.wait()
}
