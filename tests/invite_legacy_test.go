// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/netwrap"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/invite"
	"go.cryptoscope.co/ssb/sbot"
)

// first js creates an invite
// go will try to use it
func TestLegacyInviteJSCreate(t *testing.T) {
	if testutils.SkipOnCI(t) {
		return
	}

	r := require.New(t)

	os.Remove("legacy_invite.txt")

	ts := newSession(t, nil, nil)
	// TODO: configure ssbClient to use the right shs-cap
	// ts := newRandomSession(t)

	ts.startGoBot(sbot.DisableEBT(true))
	bob := ts.gobot

	wrappedAddr := bob.Network.GetListenAddr()
	// manual multiserver address
	addr := fmt.Sprintf("net:%s", netwrap.GetAddr(wrappedAddr, "tcp").String())
	addr += "~shs:"
	addr += base64.StdEncoding.EncodeToString(bob.KeyPair.ID().PubKey())
	t.Log("addr:", addr)

	bob.PublishLog.Append(map[string]interface{}{
		"type":         "address",
		"availability": 1,
		"address":      addr,
	})

	for i := 15; i > 0; i-- {
		_, err := bob.PublishLog.Publish(refs.Post{
			Type: "test-post",
			Text: fmt.Sprintf("hello, world! %d", i),
		})
		r.NoError(err)
	}

	createInvite := `
	let i = 0
	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => {
			t.comment("rpc closed:"+rpc.id)
			i++
			if (i == 2) { exit() }
		})
	})

		sbot.publish({
			type: 'about',
			about: alice.id,
			name: 'alice'
		}, 	(err, aboutMsg) => {
			t.error(err)

			sbot.invite.create({
				modern: false, // seems broken on IPv6
				note: 'testing',
				external: 'localhost'
			}, (err, invite) => {
				t.error(err)

				var fs = require('fs')
				fs.writeFile('../legacy_invite.txt', invite, (err) => {
					t.error(err)

					t.comment('ssb-js: invite saved!');
					run()
				});
			})
		})
	`

	alice := ts.startJSBotWithName("alice", createInvite, ``)

	time.Sleep(1 * time.Second)

	// prelim check of invite
	ib, err := ioutil.ReadFile("legacy_invite.txt")
	r.NoError(err)
	r.True(bytes.HasPrefix(ib, []byte("localhost:")))
	wholeInvite := string(ib)
	t.Log(wholeInvite)

	tok, err := invite.ParseLegacyToken(wholeInvite)
	r.NoError(err)
	// r.Equal("localhost", tok.Address.String()) contains...

	r.True(tok.Peer.Equal(alice), "not alice' feed!?")

	ctx := context.TODO()

	err = invite.Redeem(ctx, tok, bob.KeyPair.ID())
	r.NoError(err)

	<-ts.doneJS

	// now follow alice
	_, err = bob.PublishLog.Publish(refs.NewContactFollow(tok.Peer))
	r.NoError(err)
	bob.Replicate(alice)

	hasBobsFeed := `
	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => {

			t.comment('now should have feed:' + testBob)
			pull(
				sbot.createUserStream({id:testBob, reverse:true, limit: 4}),
				pull.collect(function(err, msgs) {
					t.error(err, 'query worked')
					t.equal(msgs.length, 4, 'got all the messages')
					sbot.publish({"type":"nice"}, (err) => {
						t.error(err, "final publish")
						setTimeout(exit, 3000)
					})
				})
			)
		})
	})
	run()
`
	ts.startJSBotWithName("alice", hasBobsFeed, ``)

	<-ts.doneJS

	/* TODO: live streaming or reconnect
	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	alicesLog, err := uf.Get(alice.StoredAddr())
	r.NoError(err)
	seqv, err := alicesLog.Seq().Value()
	r.NoError(err)
	r.EqualValues(int64(2), seqv)
	*/

	ts.wait()
}

func TestLegacyInviteJSAccept(t *testing.T) {
	r := require.New(t)

	os.Remove("legacy_invite.txt")

	// TODO: configure ssbClient to use the right shs-cap
	// ts := newRandomSession(t)
	ts := newSession(t, nil, nil)

	ts.startGoBot(sbot.DisableEBT(true))
	bob := ts.gobot

	wrappedAddr := bob.Network.GetListenAddr()
	// manual multiserver address
	addr := fmt.Sprintf("net:%s", netwrap.GetAddr(wrappedAddr, "tcp").String())
	addr += "~shs:"
	addr += base64.StdEncoding.EncodeToString(bob.KeyPair.ID().PubKey())
	t.Log("addr:", addr)

	bob.PublishLog.Append(map[string]interface{}{
		"type":         "address",
		"availability": 1,
		"address":      addr,
	})

	for i := 15; i > 0; i-- {
		_, err := bob.PublishLog.Publish(refs.Post{
			Type: "test-post",
			Text: fmt.Sprintf("hello, world! %d", i),
		})
		r.NoError(err)
	}

	master, err := client.NewTCP(bob.KeyPair, wrappedAddr)
	r.NoError(err)

	var invite string
	err = master.Async(context.TODO(), &invite, muxrpc.TypeString, muxrpc.Method{"invite", "create"})
	r.NoError(err)
	t.Log(invite)

	acceptInvite := fmt.Sprintf(`
		sbot.on('rpc:connect', (rpc) => {
			rpc.on('closed', () => {
				pull(
					sbot.createUserStream({id:testBob, reverse:true, limit: 4}),
					pull.collect(function(err, msgs) {
						t.error(err, 'query worked')
						t.equal(msgs.length, 4, 'got all the messages')
						exit()
					})
				)
			})
		})

		let inv = %q
		t.comment(inv)

		sbot.invite.accept(inv, (err, result) => {
			t.error(err)
			t.comment(JSON.stringify(result))
			run()
		});
			`, invite)

	ts.startJSBotWithName("alice", acceptInvite, ``)

	<-ts.doneJS

	ts.wait()
}
