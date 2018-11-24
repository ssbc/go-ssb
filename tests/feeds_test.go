package tests

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

func TestFeedFromJS(t *testing.T) {
	r := require.New(t)
	const n = 64
	bob, alice, done, cleanup := initInterop(t, `
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 64
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", i:i}))
	}
	series(msgs, function(err, results) {
		t.error(err, "series of publish")
		t.equal(n, results.length, "message count")
		run() // triggers connect and after block
	})
`, `
pull(
	sbot.createUserStream({id:alice.id}),
	pull.collect(function(err, vals){
		t.equal(n, vals.length)
		t.error(err, "collect")
		setTimeout(exit, 1500) // give go a chance to get this
	})
)
`)
	defer cleanup()
	<-done

	aliceLog, err := bob.UserFeeds.Get(librarian.Addr(alice.ID))
	r.NoError(err)
	seq, err := aliceLog.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(n-1), seq)

	var lastMsg string
	for i := 0; i < n; i++ {
		// only one feed in log - directly the rootlog sequences
		seqMsg, err := aliceLog.Get(margaret.BaseSeq(i))
		r.NoError(err)
		r.Equal(seqMsg, margaret.BaseSeq(i))

		msg, err := bob.RootLog.Get(seqMsg.(margaret.BaseSeq))
		r.NoError(err)
		storedMsg, ok := msg.(message.StoredMessage)
		r.True(ok, "wrong type of message: %T", msg)
		r.Equal(storedMsg.Sequence, margaret.BaseSeq(i+1))

		type testWrap struct {
			Author  ssb.FeedRef
			Content struct {
				Type, Text string
				I          int
			}
		}
		var m testWrap
		err = json.Unmarshal(storedMsg.Raw, &m)
		r.NoError(err)
		r.Equal(alice.ID, m.Author.ID, "wrong author")
		r.Equal(m.Content.Type, "test")
		r.Equal(m.Content.Text, "foo")
		r.Equal(m.Content.I, n-i, "wrong I on msg: %d", i)
		if i == n-1 {
			lastMsg = storedMsg.Key.Ref()
		}
	}

	before := fmt.Sprintf(`fromKey = %q // global - pubKey of the first alice
t.comment('shouldnt have alices feed:' + fromKey)

sbot.publish({type: 'contact', contact: fromKey, following: true}, function(err, msg) {
  t.error(err, 'follow:' + fromKey)

sbot.friends.get({src: alice.id, dest: fromKey}, function(err, val) {
  t.error(err, 'friends.get of new contact')
  t.equals(val[alice.id], true, 'is following')

pull(
  sbot.createUserStream({id:fromKey}),
  pull.collect(function(err, vals){
    t.error(err)
    t.equal(0, vals.length)
    run() // connect to go-sbot
  })
)

}) // friends.get

}) // publish`, alice.Ref())

	after := fmt.Sprintf(`t.comment('now should have feed:' + fromKey)
setTimeout(function() {
  pull(
    sbot.createUserStream({id:fromKey}),
    pull.collect(function(err, msgs) {
      t.equal(64, msgs.length, 'got all the messages')
      t.equal(%q, msgs[63].key)
      t.error(err, 'query worked')
      exit()
    })
  )
}, 3000) // wait for go to send these
`, lastMsg)

	claire, done := startJSBot(t, before, after, bob.KeyPair.Id.Ref(), netwrap.GetAddr(bob.Node.GetListenAddr(), "tcp").String())

	t.Logf("started claire: %s", claire.Ref())
	<-done

	r.NoError(bob.Close())
}
