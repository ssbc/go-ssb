// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package tests

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

// first simple case
// alice, bob, claire
// alice<>bob and bob<>claire are friends
// but alice starts blocking claire
// clair should not get new messages from alice's feed anymore
func XTestBlocking(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	const n = 23

	ts := newRandomSession(t)

	gobotRepo := repo.New(filepath.Join(ts.repo, "gobot"))
	kpAlice, err := repo.NewKeyPair(gobotRepo, "alice", refs.RefAlgoFeedSSB1)
	r.NoError(err)

	ts.startGoBot(sbot.DisableEBT(true))
	bob := ts.gobot

	_, err = bob.PublishAs("alice", refs.NewContactFollow(bob.KeyPair.ID()))
	r.NoError(err)
	aliceHelloWorld, err := bob.PublishAs("alice", refs.NewPost("hello, world!"))
	r.NoError(err)

	claire := ts.startJSBotWithName("TestBlocking/claire", fmt.Sprintf(`
	let testAlice = %q
	let testAlicePost = %q
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 23
	let msgs = []
	msgs.push(mkMsg({type:"contact", contact: testAlice, following: true}))
	msgs.push(mkMsg({type:"contact", contact: testBob, following: true}))
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", i:i}))
	}

	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', function() {
		//
		t.comment('now should have feed:' + testAlice)
		pull(
		  sbot.createUserStream({id:testAlice, reverse:true, limit: 1}),
		  pull.collect(function(err, msgs) {
			t.error(err, 'query worked')
			t.equal(msgs.length, 1, 'got all the messages')
			t.equal(msgs[0].key, testAlicePost, 'latest keys match')
			t.equal(msgs[0].value.sequence, 2, 'latest sequence')
			exit()
		  })
		)
}))

	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(msgs.length, results.length, "message count")
		run() // triggers connect and after block
	})
	`, kpAlice.ID().String(), aliceHelloWorld.Key().String()), ``)

	newSeq, err := bob.PublishLog.Append(refs.NewContactFollow(claire))
	r.NoError(err)
	r.NotNil(newSeq)
	newSeq, err = bob.PublishLog.Append(refs.NewContactFollow(kpAlice.ID()))
	r.NoError(err)
	r.NotNil(newSeq)
	bob.Replicate(claire)
	bob.Replicate(kpAlice.ID())

	<-ts.doneJS

	_, err = bob.PublishAs("alice", refs.NewContactBlock(claire))
	r.NoError(err)
	dontGet, err := bob.PublishAs("alice", refs.Post{Type: "post", Text: "post sync - who the heck is claire?"})
	r.NoError(err)

	// restart claire and connect again
	// check that C doesn't get alices new posts
	ts.startJSBotWithName("TestBlocking/claire", fmt.Sprintf(`
	let testAlice = %q
	let testAlicePost = %q
	let testAliceNowBlocked = %q

	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', function() {
		//
		t.comment('still only up to 2?')
		pull(
		  sbot.createUserStream({id:testAlice, reverse:true, limit: 1}),
		  pull.collect(function(err, msgs) {
			t.error(err, 'query worked')
			t.equal(msgs.length, 1, 'got all the messages')
			t.equal(msgs[0].key, testAlicePost, 'latest keys match')
			t.notEqual(msgs[0].key, testAliceNowBlocked, 'shouldnt have message after block!')
			t.equal(msgs[0].value.sequence, 2, 'latest sequence')
			exit()
		  })
		)
	}))

	pull(
		sbot.createUserStream({id:testAlice}),
		pull.collect(function(err, msgs) {
			t.error(err, 'init query worked')
			t.equal(msgs.length, 2, 'should have previous messages')
			run()
		})
	)
	`,
		kpAlice.ID().String(),
		aliceHelloWorld.Key().String(),
		dontGet.Key().String(),
	), ``)

	ts.wait()
}
