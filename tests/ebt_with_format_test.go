// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: CC0-1.0

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/query"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

// This test spawns a JS peer (alice) that creates a metafeed
// by doing so, the js stack also creates a metafeed/announce message
// so after the initial connection we learn about this mf and the other subfeed
// in the end we should get that 2nd subfeed
// TODO:
//   * we also create subfeeds (but we don't make it so that JS receives them yet)
func TestEBTWithFormatBendy(t *testing.T) {
	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
		// TODO: expose index flushing
	}

	r := require.New(t)

	ts := newSession(t, nil, nil)
	// TODO: fix hmac support in metafeeds to enable this
	// ts:= newRandomSession(t)

	ts.startGoBot(
		sbot.WithMetaFeedMode(true),
		sbot.DisableEBT(false),
		sbot.WithPromisc(true), // ??
	)
	sbot := ts.gobot

	alice, port := ts.startJSBotAsServerDB2("alice", `

	const source = sbot.db.query(
		where(author(testBob)),
		live(true),
		toPullStream()
	)

	pull(
		source,
		pull.drain((msg) => {
			t.comment("metafeed msg: " + msg.value.author + ":" + msg.value.sequence)
			if (msg.value.sequence === 1) {

				// the subfeed was announced
				// now stream its messages
				const subfeedSrc = sbot.db.query(
					where(author(msg.value.content.subfeed)),
					live(true),
					toPullStream()
				)
				pull(
					subfeedSrc,
					pull.drain((msg) => {
						t.comment("subfeed msg: " + msg.value.author + ":" + msg.value.sequence)
						if (msg.value.sequence === 5) {
							t.comment('Got all the messages. Shutting down in 5s')
							setTimeout(exit, 5000)
						}
					}, console.warn)
				)

				t.comment('fetching go subfeed:'+msg.value.content.subfeed)
				sbot.ebt.request(msg.value.content.subfeed, true)
			}
		}, (err) => {
			console.warn('stream closed? ' + err)
		})
	)


	sbot.metafeeds.findOrCreate((err, metafeed) => {
		if (err) throw err
		// console.log(metafeed) // { seed, keys }
		const details = {
			feedpurpose: 'mytest',
			feedformat: 'classic'
		}
		const visit = f => f.feedpurpose === 'mytest' && f.feedformat === 'classic'
		
		sbot.metafeeds.findOrCreate(metafeed, visit, details, (err, subfeed) => {
			if (err) throw err

			console.warn('meta:' + metafeed.keys.id) // tell go process who our pubkey
			sbot.ebt.request(metafeed.keys.id, true)

			console.warn('subfeed:' + subfeed.keys.id)
			sbot.ebt.request(subfeed.keys.id, true)

			sbot.db.publishAs(subfeed.keys, { type: 'test', yes: true }, console.warn)

			sbot.db.publish({
				type: 'subfeeds',
				meta: metafeed.keys.id,
				sub: subfeed.keys.id
			}, console.warn)

			function mkMsg(msg) {
				return function(cb) {
					sbot.db.publishAs(subfeed.keys, msg, cb)
				}
			}
			n = 3
			let msgs = []
			for (var i = n; i>0; i--) {
				msgs.push(mkMsg({type:"test", text:"foo", i:i}))
			}

			msgs.push(mkMsg({type:"contact", "contact": testBob, "following": true}))
			parallel(msgs, (err, results) => {
				t.error(err, "parallel of publish")
				t.equal(n+1, results.length, "message count")
				ready()
			})
		})
	})
`, ``)

	exClassic, err := sbot.MetaFeeds.CreateSubFeed(sbot.KeyPair.ID(), "example-classic", refs.RefAlgoFeedSSB1)
	r.NoError(err)
	// exGabby, err := sbot.MetaFeeds.CreateSubFeed(sbot.KeyPair.ID(), "example-gabby", refs.RefAlgoFeedGabby)
	// r.NoError(err)

	for i := 0; i <= 5; i++ {
		_, err = sbot.MetaFeeds.Publish(exClassic, refs.NewPost(strconv.Itoa(i)))
		r.NoError(err)
		// _, err = sbot.MetaFeeds.Publish(exGabby, refs.NewPost(strconv.Itoa(i)))
		// r.NoError(err)
	}

	sbot.MetaFeeds.Publish(exClassic, refs.NewContactFollow(alice))
	sbot.Replicate(alice)

	wrappedAddr := netwrap.WrapAddr(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}, secretstream.Addr{PubKey: alice.PubKey()})

	connCtx, connCancel := context.WithCancel(context.TODO())
	defer connCancel()
	err = sbot.Network.Connect(connCtx, wrappedAddr)
	r.NoError(err, "connect #1 failed")

	time.Sleep(4 * time.Second)

	// load the side-channel message with the testdata (sub- and meta-feeds)
	aliceIndex, err := sbot.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)

	aliceMsgs := mutil.Indirect(sbot.ReceiveLog, aliceIndex)
	msgv, err := aliceMsgs.Get(2)
	r.NoError(err)
	msg := msgv.(refs.Message)
	r.EqualValues(3, msg.Seq(), "has latest from alice")

	// learn about the test feeds
	var subfeeds struct {
		Type      string
		Meta, Sub refs.FeedRef
	}
	err = json.Unmarshal(msg.ContentBytes(), &subfeeds)
	r.NoError(err)
	r.Equal("subfeeds", subfeeds.Type, "expected different type on message")
	r.False(subfeeds.Sub.Equal(alice), "the subfeed isn't alice")

	// check that we want the metafeed
	replSet := sbot.Replicator.Lister().ReplicationList()
	r.True(replSet.Has(subfeeds.Meta), "replication list should have the metafeed: %w\n %s", subfeeds.Meta.ShortSigil())

	// reconnect (to restart ebt and bypass issue part 1 of #75)
	sbot.Network.GetConnTracker().CloseAll()
	connCancel()

	connCtx2, connCancel2 := context.WithCancel(context.TODO())
	defer connCancel2()
	err = sbot.Network.Connect(connCtx2, wrappedAddr)
	r.NoError(err)
	time.Sleep(4 * time.Second)

	// check we got the latest from the metafeed
	metaIndex, err := sbot.Users.Get(storedrefs.Feed(subfeeds.Meta))
	r.NoError(err)
	metaMessages := mutil.Indirect(sbot.ReceiveLog, metaIndex)
	_, err = metaMessages.Get(1)
	r.NoError(err)

	time.Sleep(3 * time.Second)

	// subfeed should be on the replicated list
	replSet = sbot.Replicator.Lister().ReplicationList()
	r.True(replSet.Has(subfeeds.Sub), "expected %s in replication list", subfeeds.Sub.ShortSigil())

	t.Log("closing bots")
	ts.wait()
}

func TestEBTWithFormatIndexed(t *testing.T) {
	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
		// TODO: expose index flushing
	}

	r := require.New(t)

	ts := newSession(t, nil, nil)
	// TODO: fix hmac support in metafeeds to enable this
	// ts:= newRandomSession(t)

	// make sure our gobot still has a normal keypair (to make publishinging simpler)
	sbotKp, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	ts.startGoBot(
		sbot.WithKeyPair(sbotKp),
		sbot.WithMetaFeedMode(true),
		sbot.DisableEBT(false),
		sbot.WithPromisc(true), // ??
	)
	sbot := ts.gobot

	alice, port := ts.startJSBotAsServerDB2("alice", `
	function mkMsg(msg) {
		return function(cb) {
			sbot.db.publish(msg, cb)
		}
	}

	sbot.indexFeedWriter.start({
		author: alice,
		type: 'dog',
		private: false,
		}, (err, idxfeed) => {
			const aliceIndexId = idxfeed.subfeed
			sbot.ebt.request(idxfeed.metafeed, true, 'bendybutt-v1')
			sbot.ebt.request(aliceIndexId, true, 'indexed')
			// actually don't want this but somehow can't replicate indexed
			sbot.ebt.request(aliceIndexId, true)
		
			let msgs = [
				mkMsg({type:"contact", "contact": testBob, "following": true}),
				mkMsg({
					type:"test",
					running: true,
					metafeed: idxfeed.metafeed,
					indexed: aliceIndexId
				}),
				// these are our two test messages
				mkMsg({type:"dog", big: true}),
				mkMsg({type:"dog", big: false}),
			]
			
			parallel(msgs, (err) => {
				t.error(err)
				t.comment(aliceIndexId)
				ready()
			})
	}) // idx.start


	// wait until go-sbot tells us they are done
	pull(
		sbot.db.query(
			where(
				and(author(testBob), type("done"))
			),
			live(true),
			toPullStream()
		),
		pull.drain((msg) => {
			t.comment(JSON.stringify(msg.value))
			t.comment("got done message, shutting down in 3s")
			setTimeout(exit, 3000)
		})
	)
	`, ``)

	// allow and fetch alice's feed
	sbot.Replicate(alice)

	// setup a sleepless wait for the messages from alice
	gotMsg := make(chan struct{})

	alicesFeed, err := sbot.Users.Get(storedrefs.Feed(alice))
	r.NoError(err)

	last := time.Now()
	done := alicesFeed.Changes().Register(
		luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			if err != nil {
				t.Log("changes err:", err)
				return err
			}

			seq, ok := v.(int64)
			if !ok {
				t.Logf("wrong seq type: %T", v)
				return nil
			}

			if seq == -1 {
				return nil
			}
			t.Logf("new message by author: %d (took %v)", seq, time.Since(last))
			last = time.Now()

			if seq == 5 {
				close(gotMsg)
			}

			return nil
		}),
	)

	aliceAddr := netwrap.WrapAddr(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}, secretstream.Addr{PubKey: alice.PubKey()})

	connCtx, connCancel := context.WithCancel(context.TODO())
	defer connCancel()
	err = sbot.Network.Connect(connCtx, aliceAddr)
	r.NoError(err, "connect #1 failed")

	select {
	case <-time.After(15 * time.Second):
		t.Fatal("timeout")
	case <-gotMsg:
	}
	done()

	// we now got the alices full feed.
	// now lookup the type:test message for the IDs of the index- and meta-feed

	sp := query.NewSubsetPlaner(sbot.Users, sbot.ByType)
	qry := query.NewSubsetAndCombination(
		query.NewSubsetOpByAuthor(alice),
		query.NewSubsetOpByType("test"),
	)

	msgs, err := sp.QuerySubsetMessages(sbot.ReceiveLog, qry)
	r.NoError(err)
	r.GreaterOrEqual(len(msgs), 1, "expected at least one message")
	t.Log(msgs[0].Key().String())

	var testMsg struct {
		Type     string
		Metafeed refs.FeedRef
		Indexed  refs.FeedRef
	}
	err = json.Unmarshal(msgs[0].ContentBytes(), &testMsg)
	r.NoError(err)
	r.Equal("test", testMsg.Type)

	// request those two feeds
	t.Log("metafeed:", testMsg.Metafeed.String())
	t.Log("index feed:", testMsg.Indexed.String())
	sbot.Replicate(testMsg.Metafeed)
	sbot.Replicate(testMsg.Indexed)
	sbot.Network.GetConnTracker().CloseAll()

	// setup sleeper for the index feed
	alicesIndexFeed, err := sbot.Users.Get(storedrefs.Feed(testMsg.Indexed))
	r.NoError(err)

	gotMsg = make(chan struct{})
	done = alicesIndexFeed.Changes().Register(
		luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			if err != nil {
				t.Log("changes err:", err)
				return err
			}

			seq, ok := v.(int64)
			if !ok {
				t.Logf("wrong seq type: %T", v)
				return nil
			}

			if seq == -1 {
				return nil
			}
			t.Logf("new message by index feed: %d (took %v)", seq, time.Since(last))
			last = time.Now()

			if seq == 1 {
				close(gotMsg)
			}

			return nil
		}),
	)

	// reconnect (current go-ebt defficiency)
	connCtx2, connCancel2 := context.WithCancel(context.TODO())
	defer connCancel2()
	err = sbot.Network.Connect(connCtx2, aliceAddr)
	r.NoError(err, "connect #2 failed")

	select {
	case <-time.After(15 * time.Second):
		t.Fatal("timeout #2")
	case <-gotMsg:
	}
	done()

	// make sure we got those indexfeed messages
	qry2 := query.NewSubsetAndCombination(
		query.NewSubsetOpByAuthor(testMsg.Indexed),
		query.NewSubsetOpByType("metafeed/index"),
	)

	indexedMsgs, err := sp.QuerySubsetMessages(sbot.ReceiveLog, qry2)
	r.NoError(err)
	r.Equal(len(indexedMsgs), 2, "expected at least one message")

	// check the messages for consistency
	var idxMsg1, idxMsg2 ssb.IndexedMessage
	err = json.Unmarshal(indexedMsgs[0].ContentBytes(), &idxMsg1)
	r.NoError(err)
	r.EqualValues(5, idxMsg1.Indexed.Sequence)

	idxedMsg1, err := sbot.Get(idxMsg1.Indexed.Key)
	r.NoError(err)
	r.EqualValues(5, idxedMsg1.Seq())
	r.True(idxedMsg1.Key().Equal(idxMsg1.Indexed.Key))

	err = json.Unmarshal(indexedMsgs[1].ContentBytes(), &idxMsg2)
	r.NoError(err)
	r.EqualValues(6, idxMsg2.Indexed.Sequence)

	idxedMsg2, err := sbot.Get(idxMsg2.Indexed.Key)
	r.NoError(err)
	r.EqualValues(6, idxedMsg2.Seq())
	r.True(idxedMsg2.Key().Equal(idxMsg2.Indexed.Key))

	// signal alice that they can shut down
	sbot.PublishLog.Publish(map[string]interface{}{
		"type": "done",
		"done": true,
	})

	// start a 2nd JS bot (claire) which fetches that index feed from the go bot
	clairesScript := fmt.Sprintf(`
	const aliceMetafeed = %q
	const alicesDogIndex = %q
	
	sbot.ebt.request(aliceMetafeed, true, 'bendybutt-v1')
	
	pull(
		sbot.db.query(
			where(author(aliceMetafeed)),
			live(true),
			toPullStream()
		),
		pull.drain((msg) => {
			t.comment(JSON.stringify(msg.value))
			if (msg.value.sequence == 1) {
				t.comment('requesting alices index feed: ' + alicesDogIndex)
				sbot.ebt.request(alicesDogIndex, true, 'indexed')
			}
		})
	)

	pull(
		sbot.db.query(
			where(author(alicesDogIndex)),
			live(true),
			toPullStream()
		),
		pull.drain((msg) => {
			t.comment(JSON.stringify(msg.value))
			if (msg.value.sequence == 2) {
				exit()
			}
		})
	)
	sbot.db.publish({type:"ready", ready:true}, ready)
	
	`, testMsg.Metafeed.String(), testMsg.Indexed.String())

	claire, clairePort := ts.startJSBotAsServerDB2("claire", clairesScript, "")

	claireAddr := netwrap.WrapAddr(&net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: clairePort,
	}, secretstream.Addr{PubKey: claire.PubKey()})

	sbot.Replicate(claire)
	time.Sleep(1 * time.Second)

	connCtx3, connCancel3 := context.WithCancel(context.TODO())
	defer connCancel3()
	err = sbot.Network.Connect(connCtx3, claireAddr)
	r.NoError(err, "connect #3 failed")

	time.Sleep(5 * time.Second)
	t.Log("closing bots")
	ts.wait()
}
