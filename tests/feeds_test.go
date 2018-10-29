package tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

func TestFeedFromJS(t *testing.T) {
	r := require.New(t)
	const n = 64
	s, alice, done, cleanup := initInterop(t, `
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
		t.end(err)
		setTimeout(exit, 1500) // give go a chance to get this
	})
)
`)
	defer cleanup()
	<-done

	aliceLog, err := s.UserFeeds.Get(librarian.Addr(alice.ID))
	r.NoError(err)
	seq, err := aliceLog.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(n-1), seq)

	for i := 0; i < n; i++ {
		// only one feed in log - directly the rootlog sequences
		seqMsg, err := aliceLog.Get(margaret.BaseSeq(i))
		r.NoError(err)
		r.Equal(seqMsg, margaret.BaseSeq(i))

		msg, err := s.RootLog.Get(seqMsg.(margaret.BaseSeq))
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
	}
	r.NoError(s.Close())
}
