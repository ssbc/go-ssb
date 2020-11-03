package tests

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	refs "go.mindeco.de/ssb-refs"
)

const createSomeMsgs = `
function mkMsg(msg) {
	return function(cb) {
		sbot.publish(msg, cb)
	}
}
n = 25
let msgs = []
for (var i = n; i>0; i--) {
	msgs.push(mkMsg({type:"test", text:"foo", i:i}))
}
msgs.push(mkMsg({type:"contact", "contact": testBob, "following": true}))
parallel(msgs, function(err, results) {
	t.error(err, "parallel of publish")
	t.equal(n+1, results.length, "message count")
	ready() // triggers connect and after block
})
`

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
	alice, port := ts.startJSBotAsServer("alice", createSomeMsgs, `t.comment("ohai");setTimeout(exit,10000)`)

	sbot.Replicate(alice)

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": sbot.KeyPair.Id.Ref(),
			"name":  "test user",
		},
		refs.NewContactFollow(alice),
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
		newSeq, err := sbot.PublishLog.Publish(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	// time.Sleep(1 * time.Second)
	// g, err := sbot.GraphBuilder.Build()
	// t.Log("build:", err)
	// err = g.RenderSVGToFile(fmt.Sprintf("%s.svg", t.Name()))
	// t.Log("render:", err)

	wrappedAddr := netwrap.WrapAddr(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}, secretstream.Addr{PubKey: alice.ID})

	err := sbot.Network.Connect(context.TODO(), wrappedAddr)
	r.NoError(err, "connect failed")

	ts.wait()
}
