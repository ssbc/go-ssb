package tests

import (
	"encoding/json"
	"fmt"
	"testing"

	"go.cryptoscope.co/ssb"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/private"
	refs "go.mindeco.de/ssb-refs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/storedrefs"
)

func TestGroupsJSCreate(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	const n = 24 + 3 // m*spam + (create, contact, invite, hint)

	ts := newRandomSession(t)

	ts.startGoBot()
	bob := ts.gobot

	// just for keygen, needed later
	claireKp, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	claire := claireKp.Id
	// claire := ts.startJSBotWithName("claire", `exit()`, "")

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
	
	let welcome = {
		text: 'this is a test group'
	}
	sbot.tribes.invite(groupId, [testBob, claireRef], welcome, (err, msg) => {
		t.error(err, 'invite worked')
		msgs.push(mkMsg({type: 'test-hint', invite: msg.key, groupId}))
		parallel(msgs, function(err, results) {
			t.error(err, "parallel of publish")
			t.equal(msgs.length, results.length, "message count")
			run() // triggers connect and after block
		})
	})

}) // tribes.create


	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))


`, claire.Ref()), ``)

	bob.Replicate(alice)
	dmKey, err := bob.Groups.GetOrDeriveKeyFor(alice)
	r.NoError(err)
	r.Len(dmKey, 1)

	dmKey, err = bob.Groups.GetOrDeriveKeyFor(claire)
	r.NoError(err)
	r.Len(dmKey, 1)

	<-ts.doneJS

	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err := uf.Get(storedrefs.Feed(alice))
	r.NoError(err)
	seq, err := aliceLog.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(n), seq)

	bob.Network.Close()

	// testutils.StreamLog(t, bob.ReceiveLog)

	hintSeqs, err := bob.ByType.Get(librarian.Addr("test-hint"))
	r.NoError(err)

	hints := mutil.Indirect(bob.ReceiveLog, hintSeqs)
	seq, err = hints.Seq().Value()
	r.NoError(err)
	firstMsg := margaret.BaseSeq(0)
	r.Equal(firstMsg, seq)

	testHintV, err := hints.Get(firstMsg)
	r.NoError(err)

	testHint := testHintV.(refs.Message)

	var hintContent struct {
		Invite  *refs.MessageRef
		GroupID string
	}
	err = json.Unmarshal(testHint.ContentBytes(), &hintContent)
	r.NoError(err)
	r.NotNil(hintContent.Invite)

	t.Log("hint:", hintContent.Invite.Ref())
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

	cloaked, err := bob.Groups.Join(ga.GroupKey, ga.Root)
	r.NoError(err)
	assert.Equal(t, hintContent.GroupID, cloaked.Ref(), "wrong derived cloaked id")

	ts.wait()
}
