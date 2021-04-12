package private_test

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
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.mindeco.de/log"
	kitlog "go.mindeco.de/log"
	"go.mindeco.de/log/level"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

/* TODO: this is an integration test and should be moved to the sbot package

before that, the indexing re-write needs to happen.
*/

func TestGroupsManualDecrypt(t *testing.T) {
	r := require.New(t)
	// a := assert.New(t)

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
	bs := botServer{todoCtx, srvLog}

	// create one bot
	srhKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("sarah"), 8)))
	r.NoError(err)

	srh, err := sbot.New(
		sbot.WithContext(ctx),
		sbot.WithKeyPair(srhKey),
		sbot.WithInfo(srvLog),
		sbot.WithInfo(log.With(srvLog, "peer", "srh")),
		sbot.WithRepoPath(filepath.Join(testRepo, "srh")),
		sbot.WithListenAddr(":0"),
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

	t.Log(cloaked.Ref(), "\nroot:", groupTangleRoot.Ref())

	suffix := []byte(".box2\"")

	// make sure this is an encrypted message
	msg, err := srh.Get(*groupTangleRoot)
	r.NoError(err)

	// can we decrypt it?
	clear, err := srh.Groups.DecryptBox2Message(msg)
	r.NoError(err)
	t.Log(string(clear))

	// publish a message to the group
	postRef, err := srh.Groups.PublishPostTo(cloaked, "just a small test group!")
	r.NoError(err)
	t.Log("post", postRef.ShortRef())

	// make sure this is an encrypted message
	msg, err = srh.Get(*postRef)
	r.NoError(err)
	content := msg.ContentBytes()
	r.True(bytes.HasSuffix(content, suffix), "%q", content)

	// create a 2nd bot
	tal, err := sbot.New(
		sbot.WithContext(ctx),
		sbot.WithInfo(log.With(srvLog, "peer", "tal")),
		sbot.WithRepoPath(filepath.Join(testRepo, "tal")),
		sbot.WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(tal))

	// hello, world! from bot2
	_, err = tal.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "shalom!"})
	r.NoError(err)
	tal.PublishLog.Publish(refs.NewContactFollow(srh.KeyPair.Id))

	// setup dm-key for bot2
	dmKey, err := srh.Groups.GetOrDeriveKeyFor(tal.KeyPair.Id)
	r.NoError(err, "%+v", err)
	r.NotNil(dmKey)
	r.Len(dmKey, 1)
	r.Len(dmKey[0].Key, 32)

	// add bot2 to the new group
	addMsgRef, err := srh.Groups.AddMember(cloaked, tal.KeyPair.Id, "welcome, tal!")
	r.NoError(err)
	t.Log("added:", addMsgRef.ShortRef())

	// it's an encrypted message
	msg, err = srh.Get(*addMsgRef)
	r.NoError(err)
	r.True(bytes.HasSuffix(msg.ContentBytes(), suffix), "%q", content)

	// have bot2 derive a key for bot1, they should be equal
	dmKey2, err := tal.Groups.GetOrDeriveKeyFor(srh.KeyPair.Id)
	r.NoError(err)
	r.Len(dmKey2, 1)
	r.Equal(dmKey[0].Key, dmKey2[0].Key)

	// now replicate a bit
	srh.Replicate(tal.KeyPair.Id)
	tal.Replicate(srh.KeyPair.Id)
	err = srh.Network.Connect(ctx, tal.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)

	// some length checks
	srhsFeeds, ok := srh.GetMultiLog("userFeeds")
	r.True(ok)
	srhsCopyOfTal, err := srhsFeeds.Get(storedrefs.Feed(tal.KeyPair.Id))
	r.NoError(err)

	talsFeeds, ok := tal.GetMultiLog("userFeeds")
	r.True(ok)
	talsCopyOfSrh, err := talsFeeds.Get(storedrefs.Feed(srh.KeyPair.Id))
	r.NoError(err)

	// did we get the expected number of messages?
	getSeq := func(l margaret.Log) int64 {
		sv, err := l.Seq().Value()
		r.NoError(err)

		seq, ok := sv.(margaret.Seq)
		r.True(ok, "wrong seq type: %T", sv)

		return seq.Seq()
	}

	r.EqualValues(1, getSeq(srhsCopyOfTal))
	r.EqualValues(3, getSeq(talsCopyOfSrh))

	// check messages can be decrypted
	addMsgCopy, err := tal.Get(*addMsgRef)
	r.NoError(err)
	content = addMsgCopy.ContentBytes()
	r.True(bytes.HasSuffix(content, suffix), "%q", content)
	t.Log(string(content))

	decr, err := tal.Groups.DecryptBox2Message(addMsgCopy)
	r.NoError(err)
	t.Log(string(decr))

	var ga private.GroupAddMember
	err = json.Unmarshal(decr, &ga)
	r.NoError(err)
	t.Logf("%x", ga.GroupKey)

	cloaked2, err := tal.Groups.Join(ga.GroupKey, ga.Root)
	r.NoError(err)
	r.Equal(cloaked.Hash, cloaked2.Hash, "cloaked ID not equal")

	// post back to group
	reply, err := tal.Groups.PublishPostTo(cloaked, fmt.Sprintf("thanks [@sarah](%s)!", srh.KeyPair.Id.Ref()))
	r.NoError(err, "tal failed to publish to group")
	t.Log("reply:", reply.ShortRef())

	// reconnect to get the reply
	edp, has := srh.Network.GetEndpointFor(*tal.KeyPair.Id)
	r.True(has)
	edp.Terminate()
	time.Sleep(1 * time.Second)
	err = srh.Network.Connect(ctx, tal.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)

	r.EqualValues(2, getSeq(srhsCopyOfTal))

	replyMsg, err := srh.Get(*reply)
	r.NoError(err)

	replyContent, err := srh.Groups.DecryptBox2Message(replyMsg)
	r.NoError(err)
	t.Log("decrypted reply:", string(replyContent))

	// indexed?
	chkCount := func(ml *roaring.MultiLog) func(tipe librarian.Addr, cnt int) {
		return func(tipe librarian.Addr, cnt int) {
			posts, err := ml.Get(tipe)
			r.NoError(err)

			pv, err := posts.Seq().Value()
			r.NoError(err)
			r.EqualValues(cnt-1, pv, "margaret is 0-indexed (%d)", cnt)

			bmap, err := ml.LoadInternalBitmap(tipe)
			r.NoError(err)
			t.Logf("%q: %s", tipe, bmap.String())
		}
	}

	chkCount(srh.ByType)("string:test", 2)
	chkCount(srh.ByType)("string:post", 2)

	chkCount(tal.ByType)("string:test", 2)
	chkCount(tal.ByType)("string:post", 1) // TODO: reindex

	addr := librarian.Addr("box2:") + storedrefs.Feed(srh.KeyPair.Id)
	chkCount(srh.Private)(addr, 3)

	addr = librarian.Addr("box2:") + storedrefs.Feed(tal.KeyPair.Id)
	chkCount(tal.Private)(addr, 2) // TODO: reindex

	/*
		t.Log("srh")
		testutils.StreamLog(t, srh.ReceiveLog)
		t.Log("tal")
		testutils.StreamLog(t, tal.ReceiveLog)
	*/

	addr = librarian.Addr("meta:box2")
	allBoxed, err := tal.Private.LoadInternalBitmap(addr)
	r.NoError(err)
	t.Log("all boxed:", allBoxed.String())

	addr = librarian.Addr("box2:") + storedrefs.Feed(tal.KeyPair.Id)
	readable, err := tal.Private.LoadInternalBitmap(addr)
	r.NoError(err)

	allBoxed.AndNot(readable)

	if n := allBoxed.GetCardinality(); n > 0 {
		t.Errorf("still have boxed messages that are not indexed: %d", n)
		t.Log("still boxed:", allBoxed.String())
	}

	time.Sleep(10 * time.Second)
	tal.Shutdown()
	srh.Shutdown()

	r.NoError(tal.Close())
	r.NoError(srh.Close())
	r.NoError(botgroup.Wait())
}

type botServer struct {
	ctx context.Context
	log kitlog.Logger
}

func (bs botServer) Serve(s *sbot.Sbot) func() error {
	return func() error {
		err := s.Network.Serve(bs.ctx)
		if err != nil {
			if err == context.Canceled {
				return nil
			}
			level.Warn(bs.log).Log("event", "bot serve exited", "err", err)
		}
		return err
	}
}

func TestGroupsReindex(t *testing.T) {
	r := require.New(t)

	// indexed?
	chkCount := func(ml *roaring.MultiLog) func(tipe librarian.Addr, cnt int) {
		return func(tipe librarian.Addr, cnt int) {
			posts, err := ml.Get(tipe)
			r.NoError(err)

			bmap, err := ml.LoadInternalBitmap(tipe)
			r.NoError(err)
			t.Logf("%q: %s", tipe, bmap.String())

			pv, err := posts.Seq().Value()
			r.NoError(err)
			r.EqualValues(cnt-1, pv, "expected more messages in multilog %q", tipe)
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
	bs := botServer{todoCtx, srvLog}

	// make the keys deterministic (helps to know who is who in the console output)
	srhKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("sarah"), 8)))
	r.NoError(err)
	talKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("tal0"), 8)))
	r.NoError(err)
	razKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("raziel"), 8)))
	r.NoError(err)

	// create the first bot
	srh, err := sbot.New(
		sbot.WithContext(ctx),
		sbot.WithKeyPair(srhKey),
		sbot.WithInfo(srvLog),
		sbot.WithInfo(log.With(srvLog, "peer", "srh")),
		sbot.WithRepoPath(filepath.Join(testRepo, "srh")),
		sbot.WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(srh))
	t.Log("srh is:", srh.KeyPair.Id.Ref())

	// just a simple paintext message
	_, err = srh.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "hello, world!"})
	r.NoError(err)

	// create a new group
	cloaked, groupTangleRoot, err := srh.Groups.Create("hello, my group")
	r.NoError(err)
	r.NotNil(groupTangleRoot)

	t.Log(cloaked.Ref(), "\nroot:", groupTangleRoot.Ref())

	// publish a message to the group
	for i := 10; i > 0; i-- {
		postRef, err := srh.Groups.PublishPostTo(cloaked, fmt.Sprintf("some test spam %d", i))
		r.NoError(err)
		t.Logf("pre-invite spam %d: %s", i, postRef.ShortRef())
	}

	// create a 2nd bot
	tal, err := sbot.New(
		sbot.WithContext(ctx),
		sbot.WithKeyPair(talKey),
		sbot.WithInfo(log.With(srvLog, "peer", "tal")),
		sbot.WithRepoPath(filepath.Join(testRepo, "tal")),
		sbot.WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(tal))
	t.Log("tal is:", tal.KeyPair.Id.Ref())

	dmKey, err := tal.Groups.GetOrDeriveKeyFor(srh.KeyPair.Id)
	r.NoError(err)
	r.Len(dmKey, 1)

	// now invite tal now that we have some content to reindex BEFORE the invite
	invref, err := srh.Groups.AddMember(cloaked, tal.KeyPair.Id, "welcome tal!")
	r.NoError(err)
	t.Log("inviteed tal:", invref.ShortRef())

	// now replicate a bit
	srh.Replicate(tal.KeyPair.Id)
	tal.Replicate(srh.KeyPair.Id)

	err = srh.Network.Connect(ctx, tal.Network.GetListenAddr())
	r.NoError(err)

	time.Sleep(2 * time.Second) // let them sync

	chkCount(srh.ByType)("string:post", 10)
	chkCount(tal.ByType)("string:post", 10)

	// create some more msgs at tal, so that raz needs to re-index both
	tal.PublishLog.Publish(refs.NewContactFollow(srh.KeyPair.Id))
	for i := 10; i > 0; i-- {
		txt := fmt.Sprintf("WOHOOO thanks for having me %d", i)
		postRef, err := tal.Groups.PublishPostTo(cloaked, txt)
		r.NoError(err)
		t.Logf("post-invite spam %d: %s", i, postRef.ShortRef())
	}

	// we dont have live streaming yet
	t.Log("reconnecting tal<>srh")
	srh.Network.GetConnTracker().CloseAll()
	err = tal.Network.Connect(ctx, srh.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(2 * time.Second) // let them sync

	chkCount(srh.Users)(storedrefs.Feed(tal.KeyPair.Id), 11)

	raz, err := sbot.New(
		sbot.WithContext(ctx),
		sbot.WithKeyPair(razKey),
		sbot.WithInfo(log.With(srvLog, "peer", "raz")),
		sbot.WithRepoPath(filepath.Join(testRepo, "raz")),
		sbot.WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(raz))
	t.Log("raz is:", raz.KeyPair.Id.Ref())

	_, err = raz.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "hello, world!"})
	r.NoError(err)

	srh.Replicate(raz.KeyPair.Id)
	tal.Replicate(raz.KeyPair.Id)
	raz.Replicate(srh.KeyPair.Id)
	raz.Replicate(tal.KeyPair.Id)

	// TODO: do we want direct message keys with everyone that we follow?!
	dmKey, err = raz.Groups.GetOrDeriveKeyFor(srh.KeyPair.Id)
	r.NoError(err)
	r.Len(dmKey, 1)

	invref2, err := srh.Groups.AddMember(cloaked, raz.KeyPair.Id, "welcome razi!")
	r.NoError(err)
	t.Log("invited raz:", invref2.Ref())

	talsLog, err := raz.Users.Get(storedrefs.Feed(tal.KeyPair.Id))
	r.NoError(err)

	// TODO: stupid hack because somehow we dont get the feed every time.... :'(
	// related to the syncing logic, not the reindexing.
	i := 5
	for i > 0 {
		t.Log("try", i)

		err = raz.Network.Connect(ctx, srh.Network.GetListenAddr())
		r.NoError(err)
		err = raz.Network.Connect(ctx, tal.Network.GetListenAddr())
		r.NoError(err)

		time.Sleep(3 * time.Second) // let them sync
		//	raz.Network.GetConnTracker().CloseAll()

		// how many messages does raz have from tal?
		v, err := talsLog.Seq().Value()
		r.NoError(err)
		seq, ok := v.(margaret.Seq)
		r.True(ok)

		if seq.Seq() == 10 {
			t.Log("received all of tal's messages")
			break
		}

		i--
	}
	r.NotEqual(0, i, "did not get the feed in %d tries", 5)

	chkCount(srh.Users)(storedrefs.Feed(tal.KeyPair.Id), 11)
	chkCount(raz.Users)(storedrefs.Feed(tal.KeyPair.Id), 11)

	chkCount(srh.ByType)("string:post", 20)
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
