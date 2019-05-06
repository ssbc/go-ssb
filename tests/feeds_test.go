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
	"go.cryptoscope.co/ssb/multilogs"
)

func TestFeedFromJS(t *testing.T) {
	r := require.New(t)
	const n = 23
	bob, alice, done, errc, cleanup := initInterop(t, `
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

	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))

	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(n, results.length, "message count")
		run() // triggers connect and after block
	})
`, ``)

	publish, err := multilogs.OpenPublishLog(bob.RootLog, bob.UserFeeds, *bob.KeyPair)
	r.NoError(err)

	newSeq, err := publish.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   alice.Ref(),
		"following": true,
	})
	r.NoError(err, "failed to publish contact message")
	r.NotNil(newSeq)

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
		r.Equal(seqMsg, margaret.BaseSeq(i+1))

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
        t.equal(%q, msgs[0].key, 'latest keys match')
        t.equal(23, msgs[0].value.sequence, 'latest sequence')
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

	claire, done, clairErrc := startJSBot(t, before, "", bob.KeyPair.Id.Ref(), netwrap.GetAddr(bob.Node.GetListenAddr(), "tcp").String())

	t.Logf("started claire: %s", claire.Ref())
	newSeq, err = publish.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   claire.Ref(),
		"following": true,
	})
	r.NoError(err, "failed to publish 2nd contact message")
	r.NotNil(newSeq)

	<-done

	bob.Shutdown()
	r.NoError(bob.Close())

	for err := range mergeErrorChans(errc, clairErrc) {
		t.Error(err)
	}
}

func TestFeedFromGo(t *testing.T) {
	r := require.New(t)
	before := `fromKey = testBob
	
	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => { 
			t.comment('now should have feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey, reverse:true, limit: 4}),
				pull.collect(function(err, msgs) {
					t.error(err, 'query worked')
					t.equal(msgs.length, 4, 'got all the messages')
					// t.comment(JSON.stringify(msgs[0]))
					t.equal(msgs[0].value.sequence, 4, 'sequence:0')
					t.equal(msgs[1].value.sequence, 3, 'sequence:1')
					t.equal(msgs[2].value.sequence, 2, 'sequence:2')
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

	s, alice, done, errc, cleanup := initInterop(t, before, "")

	publish, err := multilogs.OpenPublishLog(s.RootLog, s.UserFeeds, *s.KeyPair)
	r.NoError(err)

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": s.KeyPair.Id.Ref(),
			"name":  "test user",
		},
		map[string]interface{}{
			"type":      "contact",
			"contact":   alice.Ref(),
			"following": true,
		},
		map[string]interface{}{
			"type": "text",
			"text": `# hello world!`,
		},
		map[string]interface{}{
			"type":  "about",
			"about": alice.Ref(),
			"name":  "test alice",
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := publish.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	defer cleanup()
	<-done

	aliceLog, err := s.UserFeeds.Get(librarian.Addr(alice.ID))
	r.NoError(err)

	seqMsg, err := aliceLog.Get(margaret.BaseSeq(1))
	r.NoError(err)
	msg, err := s.RootLog.Get(seqMsg.(margaret.BaseSeq))
	r.NoError(err)
	storedMsg, ok := msg.(message.StoredMessage)
	r.True(ok, "wrong type of message: %T", msg)
	r.Equal(storedMsg.Sequence, margaret.BaseSeq(2))

	s.Shutdown()
	r.NoError(s.Close())
	r.NoError(<-errc)
}
