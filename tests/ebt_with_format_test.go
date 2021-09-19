package tests

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2/debug"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

// This test spawns a JS peer (alice) that creates a metafeed
// by doing so, the js stack also creates a metafeed/announce message
// so after the initial connection we learn about this mf and the other subfeed
// in the end we should get that 2nd subfeed
// TODO:
//   * we also create subfeeds (but we don't make it so that JS receives them yet)
func TestEBTWithFormat(t *testing.T) {
	if os.Getenv("LIBRARIAN_WRITEALL") != "0" {
		t.Fatal("please 'export LIBRARIAN_WRITEALL=0' for this test to pass")
		// TODO: expose index flushing
	}

	r := require.New(t)

	ts := newSession(t, nil, nil)

	ts.startGoBot(
		sbot.WithMetaFeedMode(true),
		sbot.DisableEBT(false),
		sbot.WithPromisc(true),
		sbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
			dumpPath := filepath.Join(ts.repo, "muxdump")
			return debug.WrapDump(dumpPath, conn)
		}),
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
			console.warn("JS received: " + msg.value.author + ":" + msg.value.sequence)
			if (msg.value.sequence === 2) {

				const subfeedSrc = sbot.db.query(
					where(author(msg.value.content.subfeed)),
					live(true),
					toPullStream()
				)
				pull(
					subfeedSrc,
					pull.drain((msg) => {
						console.warn("JS received SUbfeed: " + msg.value.author + ":" + msg.value.sequence)
						if (msg.value.sequence === 10) {
							t.comment('Got all the messages. Shutting down in 10s')
							setTimeout(exit, 1000)
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


	sbot.metafeeds.create((err, metafeed) => {
		if (err) throw err
		// console.log(metafeed) // { seed, keys }
		const details = {
			feedpurpose: 'mytest',
			feedformat: 'classic'
		}
  
		sbot.metafeeds.create(metafeed, details, (err, subfeed) => {
			if (err) throw err

			console.warn('meta:' + metafeed.keys.id) // tell go process who our pubkey
			sbot.ebt.request(metafeed.keys.id, true)

			console.warn('subfeed:' + subfeed.keys.id)
			sbot.ebt.request(subfeed.keys.id, true)

			sbot.db.publishAs(subfeed.keys, { type: 'test', yes: true }, console.warn)
			
			// TODO: metafeed/announce?!
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

	exGabby, err := sbot.MetaFeeds.CreateSubFeed("example-gabby", refs.RefAlgoFeedGabby)
	r.NoError(err)

	exClassic, err := sbot.MetaFeeds.CreateSubFeed("example-classic", refs.RefAlgoFeedSSB1)
	r.NoError(err)

	for i := 0; i < 10; i++ {
		sbot.MetaFeeds.Publish(exGabby, refs.NewPost(strconv.Itoa(i)))
		sbot.MetaFeeds.Publish(exClassic, refs.NewPost(strconv.Itoa(i)))
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
