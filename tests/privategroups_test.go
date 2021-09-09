// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package tests

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	librarian "go.cryptoscope.co/margaret/indexes"

	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/private"
	refs "go.mindeco.de/ssb-refs"
)

// ssb-db@20 problems
func XTestGroupsJSCreate(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	const n = 24 + 4 // m*spam + (create, 2*contact, invite, hint)

	ts := newRandomSession(t)

	ts.startGoBot()
	bob := ts.gobot

	// just for keygen, needed later
	claire := ts.startJSBotWithName("claire", `exit()`, "")

	alice := ts.startJSBot(fmt.Sprintf(`
	let claireRef = %q

	sbot.tribes.create({}, (err, data) => {
		const { groupId } = data
		t.comment(groupId)

		function mkMsg(msg) {
			return function(cb) {
				sbot.publish(msg, cb)
			}
		}
		n = 24 
		let msgs = []
		for (var i = n; i>0; i--) {
			msgs.push(mkMsg({type:"test", text:"foo", i:i, "recps": [groupId]}))
		}
		msgs.push(mkMsg({type: 'contact', contact: claireRef, following: true}))
		msgs.push(mkMsg({type: 'contact', contact: testBob, following: true}))
		
		let welcome = {
			text: 'this is a test group'
		}
		sbot.tribes.invite(groupId, [testBob, claireRef], welcome, (err, msg) => {
			t.error(err, 'invite worked')
			msgs.push(mkMsg({type: 'test-hint', invite: msg.key, groupId}))
			parallel(msgs, function(err, results) {
				t.error(err, "parallel of publish")
				t.equal(msgs.length, results.length, "message count")
				run()
			})
		})
	}) // tribes.create


	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))


`, claire.String()), ``)

	bob.PublishLog.Publish(refs.NewContactFollow(alice))
	bob.PublishLog.Publish(refs.NewContactFollow(claire))

	bob.Replicate(alice)
	bob.Replicate(claire)

	dmKey, err := bob.Groups.GetOrDeriveKeyFor(alice)
	r.NoError(err)
	r.Len(dmKey, 1)

	dmKey, err = bob.Groups.GetOrDeriveKeyFor(claire)
	r.NoError(err)
	r.Len(dmKey, 1)

	<-ts.doneJS

	aliceLog, err := bob.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)

	r.EqualValues(n, aliceLog.Seq())

	bob.Network.GetConnTracker().CloseAll()

	// testutils.StreamLog(t, bob.ReceiveLog)

	hintSeqs, err := bob.ByType.Get(librarian.Addr("test-hint"))
	r.NoError(err)

	hints := mutil.Indirect(bob.ReceiveLog, hintSeqs)

	firstMsg := int64(0)
	r.EqualValues(firstMsg, hints.Seq(), "expected to have hint msg")

	testHintV, err := hints.Get(firstMsg)
	r.NoError(err)

	testHint := testHintV.(refs.Message)

	// this is a public message which just tells the go-side whice one the invite is
	var hintContent struct {
		Invite  *refs.MessageRef
		GroupID string
	}
	err = json.Unmarshal(testHint.ContentBytes(), &hintContent)
	r.NoError(err)
	r.NotNil(hintContent.Invite)

	t.Log("hint:", hintContent.Invite.String())
	inviteMsg, err := bob.Get(*hintContent.Invite)
	r.NoError(err)
	r.True(inviteMsg.Author().Equal(alice), "not from alice?!")
	r.EqualValues(2, inviteMsg.Seq(), "not from alice?!")

	decr, err := bob.Groups.DecryptBox2Message(inviteMsg)
	r.NoError(err)
	t.Log("decrypted invite:", string(decr))

	var ga private.GroupAddMember
	err = json.Unmarshal(decr, &ga)
	r.NoError(err)

	// use the add to join the group
	cloaked, err := bob.Groups.Join(ga.GroupKey, ga.Root)
	r.NoError(err)
	assert.Equal(t, hintContent.GroupID, cloaked.String(), "wrong derived cloaked id")

	// reply after joining
	helloGroup, err := bob.Groups.PublishPostTo(cloaked, "hello test group!")
	r.NoError(err)

	// now start claire and let her read bobs hello.
	ts.startJSBotWithName("claire", fmt.Sprintf(`
	let testAlice = %q
	let helloGroup = %q

	let gotInvite = false
	pull(
		sbot.createLogStream({
			keys:true,
			live:true,
			private:true
		}),
		pull.drain((msg) => {
			if (msg.sync) return
			if (typeof msg.value.content === "string") return

			if (msg.value.content.type == "group/add-member") {
				gotInvite = true
			}
		})
	)
	
	sbot.replicate.request(testAlice, true)
	sbot.replicate.request(testBob, true)
	run()

	// check if we got the stuff once bob disconnects
	sbot.on('rpc:connect', rpc => rpc.on('closed', () => {
		setTimeout(() => {
			sbot.get({private: true, id: helloGroup}, (err, msg) => {
				t.error(err, "received and read hello group msg")
				t.true(gotInvite, "got the add-member msg")
				t.equal(msg.content.text, "hello test group!", "correct text on group reply from bob")
				exit()
			})
		}, 2000)
	}))
`, alice.String(), helloGroup.String()), ``)

	bob.Network.GetConnTracker().CloseAll()

	ts.wait()
}

// ssb-db@20 problems
func XTestGroupsGoCreate(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)

	ts.startGoBot()
	bob := ts.gobot

	// just for keygen, needed later
	// or fix live-streaming
	alice := ts.startJSBotWithName("alice", `exit()`, "")

	bob.Replicate(alice)

	// TODO: fix first message bug
	bob.PublishLog.Publish(map[string]string{
		"type":  "needed",
		"hello": "world"})

	groupID, _, err := bob.Groups.Create("yet another test group")
	r.NoError(err)

	for i := 7; i > 0; i-- {
		jsonContent := fmt.Sprintf(`{"type":"test", "count":%d}`, i)
		spam, err := bob.Groups.PublishTo(groupID, []byte(jsonContent))
		r.NoError(err)
		t.Logf("%d: spam %s", i, spam.String())
	}

	inviteRef, err := bob.Groups.AddMember(groupID, alice, "what's up alice?")
	r.NoError(err)

	ts.startJSBotWithName("alice", fmt.Sprintf(`

	let inviteMsg = %q
	sbot.replicate.request(testBob, true)
	run() // connect to the go side (which already published it's group messages)

	// check if we got the stuff once bob disconnects
	sbot.on('rpc:connect', rpc => rpc.on('closed', () => {
		setTimeout(() => {
			sbot.get({private: true, id: inviteMsg}, (err, msg) => {
				t.error(err, "got hello group msg")
				t.true(msg.meta.private, 'did decrypt')
				t.equal("what's up alice?", msg.content.text, 'has the right welcome text')
				// console.warn(JSON.stringify(msg, null, 2))
				exit()
			})
		},10000) // was getting the error below
	}))
`, inviteRef.String()), ``)

	ts.wait()
}

/*
not ok 3 got hello group msg
  ---
    operator: error
    at: <anonymous> (/home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/create.js:70:25)
    stack: |-
      TypeError: flumeview-level.get: index for: %x+5CduWrrxQFvNoViY7v8r2In/9v9aIOR1Q97/7pcNM=.sha256pointed at:18247but log error
          at /home/cryptix/go-repos/ssb/tests/node_modules/flumeview-level/index.js:170:17
          at /home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/minimal.js:54:7
          at /home/cryptix/go-repos/ssb/tests/node_modules/flumelog-offset/frame/recoverable.js:48:11
        TypeError: Cannot read property 'private' of undefined
          at eval (eval at <anonymous> (/home/cryptix/go-repos/ssb/tests/sbot.js:98:3), <anonymous>:11:20)
          at /home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/create.js:85:14
          at /home/cryptix/go-repos/ssb/tests/node_modules/flumeview-level/index.js:180:15
          at /home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/minimal.js:52:7
          at /home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/minimal.js:41:35
          at AsyncJobQueue.runAll (/home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/util.js:197:32)
          at Array.<anonymous> (/home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/minimal.js:41:22)
          at chainMaps (/home/cryptix/go-repos/ssb/tests/node_modules/ssb-db/minimal.js:61:14)
          at /home/cryptix/go-repos/ssb/tests/node_modules/flumedb/index.js:119:14
          at /home/cryptix/go-repos/ssb/tests/node_modules/flumelog-offset/inject.js:93:9
  ...
*/
