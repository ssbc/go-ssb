package private_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

/* TODO: this is an integration test and should be moved to the sbot package

before that, the indexing re-write needs to happen.
*/

func TestGroupsFullCircle(t *testing.T) {
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
		sbot.LateOption(sbot.WithUNIXSocket()),
		sbot.LateOption(sbot.MountSimpleIndex("get", indexes.OpenGet)), // todo muxrpc plugin is hardcoded
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

	// helper function, closured to wrap the r-helper
	suffix := []byte(".box2\"")
	getCiphertext := func(m refs.Message) []byte {
		content := m.ContentBytes()

		r.True(bytes.HasSuffix(content, suffix), "%q", content)

		n := base64.StdEncoding.DecodedLen(len(content))
		ctxt := make([]byte, n)
		decn, err := base64.StdEncoding.Decode(ctxt, bytes.TrimSuffix(content, suffix)[1:])
		r.NoError(err)
		return ctxt[:decn]
	}

	// make sure this is an encrypted message
	msg, err := srh.Get(*groupTangleRoot)
	r.NoError(err)

	// can we decrypt it?
	clear, err := srh.Groups.DecryptBox2(getCiphertext(msg), srh.KeyPair.Id, msg.Previous())
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
		sbot.LateOption(sbot.WithUNIXSocket()),
		sbot.LateOption(sbot.MountSimpleIndex("get", indexes.OpenGet)), // todo muxrpc plugin is hardcoded
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
	srhsCopyOfTal, err := srhsFeeds.Get(tal.KeyPair.Id.StoredAddr())
	r.NoError(err)

	talsFeeds, ok := tal.GetMultiLog("userFeeds")
	r.True(ok)
	talsCopyOfSrh, err := talsFeeds.Get(srh.KeyPair.Id.StoredAddr())
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

	decr, err := tal.Groups.DecryptBox2(getCiphertext(addMsgCopy), addMsgCopy.Author(), addMsgCopy.Previous())
	r.NoError(err)
	t.Log(string(decr))

	var ga private.GroupAddMember
	err = json.Unmarshal(decr, &ga)
	r.NoError(err)
	t.Logf("%x", ga.GroupKey)

	cloaked2, err := tal.Groups.Join(ga.GroupKey, ga.InitialMessage)
	r.NoError(err)
	r.Equal(cloaked.Hash, cloaked2.Hash, "cloaked ID not equal")

	// post back to group
	reply, err := tal.Groups.PublishPostTo(cloaked, fmt.Sprintf("thanks [@sarah](%s)!", srh.KeyPair.Id.Ref()))
	r.NoError(err, "tal failed to publish to group")
	t.Log("reply:", reply.ShortRef())

	// reconnect to get the reply
	edp, has := srh.Network.GetEndpointFor(tal.KeyPair.Id)
	r.True(has)
	edp.Terminate()
	time.Sleep(1 * time.Second)
	err = srh.Network.Connect(ctx, tal.Network.GetListenAddr())
	r.NoError(err)
	time.Sleep(1 * time.Second)

	r.EqualValues(2, getSeq(srhsCopyOfTal))

	replyMsg, err := srh.Get(*reply)
	r.NoError(err)

	replyContent, err := srh.Groups.DecryptBox2(getCiphertext(replyMsg), replyMsg.Author(), replyMsg.Previous())
	r.NoError(err)
	t.Log(string(replyContent))

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

	chkCount(srh.ByType)("test", 2)
	chkCount(srh.ByType)("post", 2)

	chkCount(tal.ByType)("test", 2)
	chkCount(tal.ByType)("post", 1) // TODO: reindex

	addr := librarian.Addr("box2:") + srh.KeyPair.Id.StoredAddr()
	chkCount(srh.Private)(addr, 3)

	addr = librarian.Addr("box2:") + tal.KeyPair.Id.StoredAddr()
	chkCount(tal.Private)(addr, 2) // TODO: reindex

	t.Log("srh")
	streamLog(t, srh.RootLog)

	t.Log("tal")
	streamLog(t, tal.RootLog)

	stillBoxed, err := tal.Private.LoadInternalBitmap(librarian.Addr("notForUs:box2"))
	r.NoError(err)
	t.Log("stillBoxed:", stillBoxed.String())

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

func streamLog(t *testing.T, l margaret.Log) {

	r := require.New(t)
	src, err := l.Query()
	r.NoError(err)
	i := 0
	for {
		v, err := src.Next(context.TODO())
		if luigi.IsEOS(err) {
			break
		}

		mm, ok := v.(refs.Message)
		r.True(ok, "%T", v)

		t.Log(i, mm.Key().ShortRef())
		t.Log(mm.Author().ShortRef(), mm.Seq())

		b := mm.ContentBytes()
		if len(b) > 128 {
			b = b[len(b)-32:]
		}
		t.Logf("\n%s", hex.Dump(b))

		i++
	}

}
