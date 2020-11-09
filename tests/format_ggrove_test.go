// SPDX-License-Identifier: MIT

package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/muxrpc/v2/codec"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestGabbyFeedFromGo(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newSession(t, nil, nil)
	// hmac not supported on the js side
	// ts := newRandomSession(t)

	kp, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	kp.Id.Algo = refs.RefAlgoFeedGabby

	ts.startGoBot(sbot.WithKeyPair(kp))
	s := ts.gobot

	before := `fromKey = testBob
	sbot.on('rpc:connect', (rpc) => {
        t.comment('got connection: ' + rpc.id)
		pull(
			rpc.createHistoryStream({id: fromKey}),
			pull.collect((err, msgs) => {
				t.error(err)
				t.equal(msgs.length,3)
				console.warn('Messages: '+msgs.length)
				// console.warn(JSON.stringify(msgs))
				sbot.gabbygrove.verify(msgs[0], (err, evt) => {
					t.error(err, 'verified msg[0]')
					t.ok(evt)
					t.comment('exiting in 3 secs')
					setTimeout(exit, 3000)
				})
			})
		)
	})

    run()

	// following is blocked on proper feed format support with new suffixes
`

	alice := ts.startJSBot(before, "")

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "ex-message",
			"hello": "world",
		},
		refs.NewContactFollow(alice),
		map[string]interface{}{
			"type":  "message",
			"text":  "whoops",
			"fault": true,
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := s.PublishLog.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}
	s.Replicate(alice)

	time.Sleep(1 * time.Second) // wait for alice' connection

	aliceEdp, ok := s.Network.GetEndpointFor(alice)
	r.True(ok, "no endpoint for alice")

	ctx := context.TODO()
	src, err := aliceEdp.Source(ctx, codec.Body{}, muxrpc.Method{"gabbygrove", "binaryStream"})
	r.NoError(err)

	// hacky, pretend alice is a gabby formated feed (as if it would respond to createHistoryStream)
	aliceAsGabby := *alice
	aliceAsGabby.Algo = refs.RefAlgoFeedGabby
	store := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}
		_, err = s.ReceiveLog.Append(val)
		return errors.Wrap(err, "failed to append verified message to rootLog")
	})
	snk := message.NewVerifySink(&aliceAsGabby, margaret.BaseSeq(1), nil, store, nil)

	err = luigi.Pump(ctx, snk, src)
	r.NoError(err)

	// test is currently borked because we get fake messages back

	uf, ok := s.GetMultiLog("userFeeds")
	r.True(ok)
	demoLog, err := uf.Get(storedrefs.Feed(&aliceAsGabby))
	r.NoError(err)

	demoLogSeq, err := demoLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(2, demoLogSeq.(margaret.Seq).Seq())

	for demoFeedSeq := margaret.BaseSeq(1); demoFeedSeq < 3; demoFeedSeq++ {
		seqMsg, err := demoLog.Get(demoFeedSeq - 1)
		r.NoError(err)
		msg, err := s.ReceiveLog.Get(seqMsg.(margaret.BaseSeq))
		r.NoError(err)
		storedMsg, ok := msg.(refs.Message)
		r.True(ok, "wrong type of message: %T", msg)

		var testMsg struct {
			Message string
			Level   int
		}
		err = json.Unmarshal(storedMsg.ContentBytes(), &testMsg)
		r.NoError(err)

		r.Equal(aliceAsGabby.Ref(), storedMsg.Author().Ref())

		r.Equal(demoFeedSeq.Seq(), storedMsg.Seq())
		switch demoFeedSeq {
		case 1:
			r.Equal(testMsg.Message, "hello world")
			r.Equal(testMsg.Level, 0)
		case 2:
			r.Equal(testMsg.Message, "exciting")
			r.Equal(testMsg.Level, 9000)
		case 3:
			r.Equal(testMsg.Message, "last")
			r.Equal(testMsg.Level, 9001)
		}

		t.Log("age:", time.Since(storedMsg.Received()))
	}

	ts.wait()
}
