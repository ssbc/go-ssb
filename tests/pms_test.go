// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package tests

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/private/box"
	"go.cryptoscope.co/ssb/sbot"
)

func TestPrivMsgsFromGo(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	boxer := box.NewBoxer(nil)

	ts.startGoBot(sbot.DisableEBT(true))
	s := ts.gobot

	before := `fromKey = testBob

	var isTestMessage = (msg) => {
		return msg.value.content.type === "test"
	}

	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => {
			t.comment('should now have feed:' + fromKey)
			pull(
				sbot.createLogStream({private:true, meta:true}),
				pull.filter((msg) => { return msg.value.private == true }),
				pull.filter((msg) => {
					console.warn(msg) // just log them
					return true
				}),
				pull.collect(function(err, msgs) {
					t.error(err, 'private read worked')
					t.equal(msgs.length, 5, 'got all the messages')

					t.true(msgs.every(isTestMessage), "all test messages")

					t.equal(msgs[0].value.sequence, 2, 'sequence:0')
					t.deepEqual(msgs[0].value.content.some, 1, 'sequence:0 val')

					t.equal(msgs[1].value.sequence, 3, 'sequence:1')
					t.equal(msgs[1].value.content.some, 2, 'sequence:1 val')

					t.equal(msgs[2].value.sequence, 4, 'sequence:2')
					t.equal(msgs[2].value.content.some, 3,  'sequence:2 val')

					t.equal(msgs[3].value.sequence, 5, 'sequence:3')
					t.equal(msgs[3].value.content.some, 4, 'sequence:3 val')

					t.equal(msgs[4].value.sequence, 6, 'sequence:4')
					t.equal(msgs[4].value.content.some, 5, 'sequence:4 val')
					exit()
				})
			)
		})
	})

	sbot.publish({type: 'contact', contact: fromKey, following: true}, function(err, msg) {
		t.error(err, 'follow:' + fromKey)

		sbot.friends.get({src: alice.id, dest: fromKey}, function(err, val) {
			t.error(err, 'friends.get of new contact')
			t.equals(val[alice.id], true, 'is following:'+JSON.stringify(val))

			t.comment('shouldnt have bobs feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey}),
				pull.collect(function(err, vals){
					t.error(err)
					t.equal(0, vals.length)
					sbot.publish({type: 'about', about: fromKey, name: 'test bob'}, function(err, msg) {
						t.error(err, 'about:' + msg.key)
						run()
					})
				})
			)

}) // friends.get

}) // publish`

	alice := ts.startJSBot(before, "")

	s.Replicate(alice)
	newSeq, err := s.PublishLog.Append(refs.NewContactFollow(alice))

	r.NoError(err, "failed to publish contact message")
	r.NotNil(newSeq)

	var tmsgs = [][]byte{
		[]byte(`{"some": 1, "msg": "this", "type":"test"}`),
		[]byte(`{"some": 2, "msg": "is", "type":"test"}`),
		[]byte(`{"some": 3, "msg": "NOT?", "type":"test"}`),
		[]byte(`{"some": 4, "msg": "a", "type":"test"}`),
		[]byte(`{"some": 5, "msg": "test", "type":"test"}`),
	}

	for i, msg := range tmsgs {
		sbox, err := boxer.Encrypt(msg, alice, s.KeyPair.ID())
		r.NoError(err, "failed to create ciphertext %d", i)

		newSeq, err := s.PublishLog.Append(sbox)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	<-ts.doneJS

	aliceLog, err := s.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)

	seqMsg, err := aliceLog.Get(int64(1))
	r.NoError(err)
	msg, err := s.ReceiveLog.Get(seqMsg.(int64))
	r.NoError(err)
	storedMsg, ok := msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(2, storedMsg.Seq())

	ts.wait()
}

func TestPrivMsgsFromJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	boxer := box.NewBoxer(nil)

	ts.startGoBot(sbot.DisableEBT(true))
	bob := ts.gobot

	const n = 16
	alice := ts.startJSBot(`let recps = [ testBob, alice.id ]
	function mkMsg(msg) {
		return function(cb) {
			msg["recps"] = recps
			sbot.publish(msg, cb)
		}
	}
	n = 16
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", "i":i}))
	}

    // be done when the other party is done
    sbot.on('rpc:connect', rpc => rpc.on('closed', exit))

	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(n, results.length, "message count")
		run() // triggers connect and after block
	})
`, ``)

	newSeq, err := bob.PublishLog.Append(refs.NewContactFollow(alice))
	bob.Replicate(alice)

	r.NoError(err, "failed to publish contact message")
	r.NotNil(newSeq)

	<-ts.doneJS

	aliceLog, err := bob.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)
	r.EqualValues(n-1, aliceLog.Seq())

	// var lastMsg string
	for i := 0; i < n; i++ {
		seqMsg, err := aliceLog.Get(int64(i))
		r.NoError(err)
		//r.Equal(seqMsg, int64(1+i))

		msg, err := bob.ReceiveLog.Get(seqMsg.(int64))
		r.NoError(err)
		absMsg, ok := msg.(refs.Message)
		r.True(ok, "wrong type of message: %T", msg)
		r.EqualValues(i+1, absMsg.Seq())

		if i == 0 {
			continue // skip contact
		}
		type testWrap struct {
			Author  refs.FeedRef
			Content string
		}
		var m testWrap
		err = json.Unmarshal(absMsg.ValueContentJSON(), &m)
		r.NoError(err)
		r.True(alice.Equal(m.Author), "wrong author")
		r.True(strings.HasSuffix(m.Content, ".box"), "not a boxed msg? %s", string(m.Content))

		data, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(m.Content, ".box"))
		r.NoError(err)

		clearMsg, err := boxer.Decrypt(bob.KeyPair, data)
		r.NoError(err, "should decrypt the msg %d!", i)

		type testMsg struct {
			Tipe string `json:"type"`
			I    int    `json:"i"`
		}
		var clearObj testMsg
		err = json.Unmarshal(clearMsg, &clearObj)
		r.NoError(err, "should json decode msg %d!", i)
		r.Equal(16-i, clearObj.I, "wrong count on msg %d", i)
		r.Equal("test", clearObj.Tipe)
	}

	ts.wait()
}
