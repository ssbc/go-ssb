// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.mindeco.de/log"
	kitlog "go.mindeco.de/log"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/private"
	refs "go.mindeco.de/ssb-refs"
)

func TestPrivateGroupsManualDecrypt(t *testing.T) {
	r := require.New(t)

	// cleanup previous run
	testRepo := filepath.Join("testrun", t.Name())
	os.RemoveAll(testRepo)

	// bot hosting and logging boilerplate
	srvLog := kitlog.NewNopLogger()
	if testing.Verbose() {
		srvLog = kitlog.NewLogfmtLogger(os.Stderr)
	}
	todoCtx := context.TODO()
	botgroup, ctx := errgroup.WithContext(todoCtx)
	bs := newBotServer(todoCtx, srvLog)

	// create one bot
	srhKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("sarah"), 8)), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	srh, err := New(
		WithContext(ctx),
		WithKeyPair(srhKey),
		WithInfo(srvLog),
		WithInfo(log.With(srvLog, "peer", "srh")),
		WithRepoPath(filepath.Join(testRepo, "srh")),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(srh))

	// just a simple paintext message
	_, err = srh.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "hello, world!"})
	r.NoError(err)

	// create a new group
	cloaked, groupTangleRoot, err := srh.Groups.Create("hello, my group")
	r.NoError(err)
	r.NotNil(groupTangleRoot)

	t.Log(cloaked.String(), "\nroot:", groupTangleRoot.String())

	suffix := []byte(".box2\"")

	// make sure this is an encrypted message
	msg, err := srh.Get(groupTangleRoot)
	r.NoError(err)

	// can we decrypt it?
	clear, err := srh.Groups.DecryptBox2Message(msg)
	r.NoError(err)
	t.Log(string(clear))

	// publish a message to the group
	postRef, err := srh.Groups.PublishPostTo(cloaked, "just a small test group!")
	r.NoError(err, "failed to publish post to group")
	t.Log("post", postRef.ShortSigil())

	// make sure this is an encrypted message
	msg, err = srh.Get(postRef)
	r.NoError(err)
	content := msg.ContentBytes()
	r.True(bytes.HasSuffix(content, suffix), "%q", content)

	// create a 2nd bot
	tal, err := New(
		WithContext(ctx),
		WithInfo(log.With(srvLog, "peer", "tal")),
		WithRepoPath(filepath.Join(testRepo, "tal")),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(tal))

	// hello, world! from bot2
	_, err = tal.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "shalom!"})
	r.NoError(err)
	tal.PublishLog.Publish(refs.NewContactFollow(srh.KeyPair.ID()))

	// setup dm-key for bot2
	dmKey, err := srh.Groups.GetOrDeriveKeyFor(tal.KeyPair.ID())
	r.NoError(err, "%+v", err)
	r.NotNil(dmKey)
	r.Len(dmKey, 1)
	r.Len(dmKey[0].Key, 32)

	// add bot2 to the new group
	addMsgRef, err := srh.Groups.AddMember(cloaked, tal.KeyPair.ID(), "welcome, tal!")
	r.NoError(err)
	t.Log("added:", addMsgRef.ShortSigil())

	// it's an encrypted message
	msg, err = srh.Get(addMsgRef)
	r.NoError(err)
	r.True(bytes.HasSuffix(msg.ContentBytes(), suffix), "%q", content)

	// have bot2 derive a key for bot1, they should be equal
	dmKey2, err := tal.Groups.GetOrDeriveKeyFor(srh.KeyPair.ID())
	r.NoError(err)
	r.Len(dmKey2, 1)
	r.Equal(dmKey[0].Key, dmKey2[0].Key)

	// now replicate a bit
	srh.Replicate(tal.KeyPair.ID())
	tal.Replicate(srh.KeyPair.ID())
	err = srh.Network.Connect(ctx, tal.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)

	// some length checks
	srhsFeeds, ok := srh.GetMultiLog("userFeeds")
	r.True(ok)
	srhsCopyOfTal, err := srhsFeeds.Get(storedrefs.Feed(tal.KeyPair.ID()))
	r.NoError(err)

	talsFeeds, ok := tal.GetMultiLog("userFeeds")
	r.True(ok)
	talsCopyOfSrh, err := talsFeeds.Get(storedrefs.Feed(srh.KeyPair.ID()))
	r.NoError(err)

	// did we get the expected number of messages?
	r.EqualValues(1, srhsCopyOfTal.Seq())
	r.EqualValues(5, talsCopyOfSrh.Seq())

	// check messages can be decrypted
	addMsgCopy, err := tal.Get(addMsgRef)
	r.NoError(err)
	content = addMsgCopy.ContentBytes()
	r.True(bytes.HasSuffix(content, suffix), "%q", content)

	decr, err := tal.Groups.DecryptBox2Message(addMsgCopy)
	r.NoError(err)
	t.Log("decrypted:", string(decr))

	var ga private.GroupAddMember
	err = json.Unmarshal(decr, &ga)
	r.NoError(err)
	t.Logf("%x", ga.GroupKey)

	cloaked2, err := tal.Groups.Join(ga.GroupKey, ga.Root)
	r.NoError(err)
	r.True(cloaked.Equal(cloaked2), "cloaked ID not equal")

	// post back to group
	reply, err := tal.Groups.PublishPostTo(cloaked, fmt.Sprintf("thanks [@sarah](%s)!", srh.KeyPair.ID().String()))
	r.NoError(err, "tal failed to publish to group")
	t.Log("reply:", reply.ShortSigil())

	// reconnect to get the reply
	edp, has := srh.Network.GetEndpointFor(tal.KeyPair.ID())
	r.True(has)
	edp.Terminate()
	time.Sleep(1 * time.Second)
	err = srh.Network.Connect(ctx, tal.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)

	r.EqualValues(2, srhsCopyOfTal.Seq())

	replyMsg, err := srh.Get(reply)
	r.NoError(err)

	replyContent, err := srh.Groups.DecryptBox2Message(replyMsg)
	r.NoError(err)
	t.Log("decrypted reply:", string(replyContent))

	// indexed?
	chkCount := func(ml *roaring.MultiLog) func(addr indexes.Addr, cnt int) {
		return func(addr indexes.Addr, cnt int) {
			posts, err := ml.Get(addr)
			r.NoError(err)

			r.EqualValues(cnt-1, posts.Seq(), "margaret is 0-indexed (%d)", cnt)

			bmap, err := ml.LoadInternalBitmap(addr)
			r.NoError(err)
			t.Logf("%q: %v", addr, bmap.ToArray())
		}
	}

	chkCount(srh.ByType)("string:test", 2)
	chkCount(srh.ByType)("string:post", 2)

	chkCount(tal.ByType)("string:test", 2)
	chkCount(tal.ByType)("string:post", 2)

	addr := indexes.Addr("box2:") + storedrefs.Feed(srh.KeyPair.ID())
	chkCount(srh.Private)(addr, 3)

	addr = indexes.Addr("box2:") + storedrefs.Feed(tal.KeyPair.ID())
	chkCount(tal.Private)(addr, 4)

	addr = indexes.Addr("meta:box2")
	allBoxed, err := tal.Private.LoadInternalBitmap(addr)
	r.NoError(err)
	t.Log("all boxed:", allBoxed.String())

	addr = indexes.Addr("box2:") + storedrefs.Feed(tal.KeyPair.ID())
	readable, err := tal.Private.LoadInternalBitmap(addr)
	r.NoError(err)

	allBoxed.AndNot(readable)

	if n := allBoxed.GetCardinality(); n > 0 {
		t.Errorf("still have boxed messages that are not indexed: %d", n)
		t.Log("still boxed:", allBoxed.String())
	}

	tal.Shutdown()
	srh.Shutdown()

	r.NoError(tal.Close())
	r.NoError(srh.Close())
	r.NoError(botgroup.Wait())
}

// TODO: somehow the Membership/Reindex functionality doesn't kick in.
// When Raz processes srh's feed, it should see the invite to tal and then index that one as well.
func XTestGroupsReindex(t *testing.T) {
	r := require.New(t)

	// indexed? asserter
	chkCount := func(ml *roaring.MultiLog) func(tipe indexes.Addr, cnt int) {
		return func(tipe indexes.Addr, cnt int) {
			posts, err := ml.Get(tipe)
			r.NoError(err)

			bmap, err := ml.LoadInternalBitmap(tipe)
			r.NoError(err)
			t.Logf("%q: %v", tipe, bmap.ToArray())

			r.EqualValues(cnt-1, posts.Seq(), "expected more messages in multilog %q", tipe)
		}
	}

	// cleanup previous run
	testRepo := filepath.Join("testrun", t.Name())
	os.RemoveAll(testRepo)

	// bot hosting and logging boilerplate
	srvLog := kitlog.NewNopLogger()
	if testing.Verbose() {
		srvLog = kitlog.NewLogfmtLogger(os.Stderr)
	}
	todoCtx := context.TODO()
	botgroup, ctx := errgroup.WithContext(todoCtx)
	bs := newBotServer(todoCtx, srvLog)

	// make the keys deterministic (helps to know who is who in the console output)
	srhKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("sarah"), 8)), refs.RefAlgoFeedSSB1)
	r.NoError(err)
	talKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("tal0"), 8)), refs.RefAlgoFeedSSB1)
	r.NoError(err)
	razKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("raziel"), 8)), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// create the first bot (who creates the group)
	srh, err := New(
		WithContext(ctx),
		WithKeyPair(srhKey),
		WithInfo(srvLog),
		WithInfo(log.With(srvLog, "peer", "srh")),
		WithRepoPath(filepath.Join(testRepo, "srh")),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(srh))
	t.Log("srh is:", srh.KeyPair.ID().String())

	// just a simple paintext message
	_, err = srh.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "hello, world!"})
	r.NoError(err)

	// srh creates a new group
	cloaked, groupTangleRoot, err := srh.Groups.Create("hello, my group")
	r.NoError(err)
	r.NotNil(groupTangleRoot)
	t.Log("new group:", cloaked.String(), "\nroot:", groupTangleRoot.String())

	// publish a few messages to the group
	for i := 1; i <= 10; i++ {
		postRef, err := srh.Groups.PublishPostTo(cloaked, fmt.Sprintf("some test spam %d", i))
		r.NoError(err)
		t.Logf("pre-invite spam %d: %s", i, postRef.ShortSigil())
	}

	// create a 2nd bot (who joins first)
	tal, err := New(
		WithContext(ctx),
		WithKeyPair(talKey),
		WithInfo(log.With(srvLog, "peer", "tal")),
		WithRepoPath(filepath.Join(testRepo, "tal")),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(tal))
	t.Log("tal is:", tal.KeyPair.ID().String())

	// make sure tal can receive srh's invite via box2-dm
	dmKey, err := tal.Groups.GetOrDeriveKeyFor(srh.KeyPair.ID())
	r.NoError(err)
	r.Len(dmKey, 1)

	// now invite tal now that we have some content to reindex BEFORE the invite to raz
	invref, err := srh.Groups.AddMember(cloaked, tal.KeyPair.ID(), "welcome tal!")
	r.NoError(err)
	t.Log("invited 1 to tal:", invref.Sigil())

	// now replicate a the two of them
	srh.Replicate(tal.KeyPair.ID())
	tal.Replicate(srh.KeyPair.ID())

	err = srh.Network.Connect(ctx, tal.Network.GetListenAddr())
	r.NoError(err)

	time.Sleep(2 * time.Second) // let them sync

	// check that they both have the messages
	chkCount(srh.ByType)("string:post", 10)
	chkCount(tal.ByType)("string:post", 10)

	// create some more msgs at tal, so that raz needs to re-index both
	tal.PublishLog.Publish(refs.NewContactFollow(srh.KeyPair.ID()))
	for i := 10; i > 0; i-- {
		txt := fmt.Sprintf("WOHOOO thanks for having me %d", i)
		postRef, err := tal.Groups.PublishPostTo(cloaked, txt)
		r.NoError(err)
		t.Logf("post-invite spam %d: %s", i, postRef.ShortSigil())
	}

	// we dont have live streaming yet
	t.Log("reconnecting tal<>srh")
	srh.Network.GetConnTracker().CloseAll()
	err = tal.Network.Connect(ctx, srh.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(2 * time.Second) // let them sync

	chkCount(srh.Users)(storedrefs.Feed(tal.KeyPair.ID()), 11)

	// create the 3rd bot who will fetch srh and tal
	// they both will have opaque messages until the key is presented to raz via invite2
	raz, err := New(
		WithContext(ctx),
		WithKeyPair(razKey),
		WithInfo(log.With(srvLog, "peer", "raz")),
		WithRepoPath(filepath.Join(testRepo, "raz")),
		WithListenAddr(":0"),
		DisableEBT(true),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(raz))
	t.Log("raz is:", raz.KeyPair.ID().String())

	_, err = raz.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "hello, world!"})
	r.NoError(err)

	srh.Replicate(raz.KeyPair.ID())
	tal.Replicate(raz.KeyPair.ID())
	raz.Replicate(srh.KeyPair.ID())
	raz.Replicate(tal.KeyPair.ID())

	// TODO: do we want direct message keys with everyone that we follow?!
	dmKey, err = raz.Groups.GetOrDeriveKeyFor(srh.KeyPair.ID())
	r.NoError(err)
	r.Len(dmKey, 1)

	invref2, err := srh.Groups.AddMember(cloaked, raz.KeyPair.ID(), "welcome razi!")
	r.NoError(err)
	t.Log("invite 2 to raz:", invref2.Sigil())

	talsLog, err := raz.Users.Get(storedrefs.Feed(tal.KeyPair.ID()))
	r.NoError(err)

	// TODO: stupid hack because somehow we dont get the feed every time.... :'(
	// related to the syncing logic, not the reindexing.
	i := 5
	for i > 0 {
		t.Log("tries left", i)
		raz.Network.GetConnTracker().CloseAll()
		time.Sleep(1 * time.Second) // let them sync

		// connect to the two other bots
		err = raz.Network.Connect(ctx, srh.Network.GetListenAddr())
		r.NoError(err)
		err = raz.Network.Connect(ctx, tal.Network.GetListenAddr())
		r.NoError(err)

		time.Sleep(5 * time.Second) // let them sync

		// how many messages does raz have from tal?
		if talsLog.Seq() == 10 {
			t.Log("received all of tal's messages")
			break
		}

		i--
	}
	r.NotEqual(0, i, "did not get the feed in %d tries", 5)

	chkCount(srh.Users)(storedrefs.Feed(tal.KeyPair.ID()), 11)
	chkCount(raz.Users)(storedrefs.Feed(tal.KeyPair.ID()), 11)

	// finally, assert they all three can read the full group
	chkCount(srh.ByType)("string:post", 20)
	chkCount(tal.ByType)("string:post", 20)
	chkCount(raz.ByType)("string:post", 20)

	// done, cleaning up
	tal.Shutdown()
	srh.Shutdown()
	raz.Shutdown()

	r.NoError(tal.Close())
	r.NoError(srh.Close())
	r.NoError(raz.Close())
	r.NoError(botgroup.Wait())
}
