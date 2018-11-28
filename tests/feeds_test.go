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
	const n = 1024
	bob, alice, done, cleanup := initInterop(t, `
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 1024
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", i:i}))
	}

    // be done when the other party is done
    sbot.on('rpc:connect', rpc => rpc.on('closed', exit))

	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(n, results.length, "message count")
		run() // triggers connect and after block
	})
`, ``)
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

sbot.on('rpc:connect', (rpc) => {
  rpc.on('closed', () => { 
    t.comment('now should have feed:' + fromKey)
    pull(
      sbot.createUserStream({id:fromKey, reverse:true, limit: 1}),
      pull.collect(function(err, msgs) {
        t.error(err, 'query worked')
        t.equal(1, msgs.length, 'got all the messages')
        t.equal(%q, msgs[0].key)
        t.equal(1024, msgs[0].value.sequence)
        exit()
      })
    )
  })
})

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

}) // publish`, alice.Ref(), lastMsg)

	claire, done := startJSBot(t, before, "", bob.KeyPair.Id.Ref(), netwrap.GetAddr(bob.Node.GetListenAddr(), "tcp").String())

	t.Logf("started claire: %s", claire.Ref())
	<-done

	r.NoError(bob.Close())
}
