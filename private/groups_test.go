package private_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/plugins2"
	"go.cryptoscope.co/ssb/plugins2/tangles"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestGroupsFullCircle(t *testing.T) {
	r := require.New(t)
	// a := assert.New(t)

	testRepo := filepath.Join("testrun", t.Name())
	os.RemoveAll(testRepo)

	srhKey, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("sarah"), 8)))
	r.NoError(err)

	srvLog := kitlog.NewNopLogger()
	if testing.Verbose() {
		srvLog = kitlog.NewLogfmtLogger(os.Stderr)
	}
	todoCtx := context.TODO()
	botgroup, ctx := errgroup.WithContext(todoCtx)
	bs := botServer{todoCtx, srvLog}

	// mlogPriv := multilogs.NewPrivateRead(kitlog.With(srvLog, "module", "privLogs"), alice)

	srh, err := sbot.New(
		sbot.WithContext(ctx),
		sbot.WithKeyPair(srhKey),
		sbot.WithInfo(srvLog),
		sbot.WithInfo(log.With(srvLog, "peer", "srh")),
		sbot.WithRepoPath(filepath.Join(testRepo, "srh")),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
		sbot.LateOption(sbot.MountSimpleIndex("get", indexes.OpenGet)), // todo muxrpc plugin is hardcoded
		sbot.LateOption(sbot.MountPlugin(&tangles.Plugin{}, plugins2.AuthMaster)),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(srh))

	_, err = srh.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "hello, world!"})
	r.NoError(err)

	cloaked, groupTangleRoot, err := srh.Groups.Init("hello, my group")
	r.NoError(err)
	r.NotNil(groupTangleRoot)

	t.Log(cloaked.Ref(), "\nroot:", groupTangleRoot.Ref())

	msg, err := srh.Get(*groupTangleRoot)
	r.NoError(err)

	content := msg.ContentBytes()

	suffix := []byte(".box2\"")
	r.True(bytes.HasSuffix(content, suffix), "%q", content)

	n := base64.StdEncoding.DecodedLen(len(content))
	ctxt := make([]byte, n)
	decn, err := base64.StdEncoding.Decode(ctxt, bytes.TrimSuffix(content, suffix)[1:])
	r.NoError(err)
	ctxt = ctxt[:decn]

	clear, err := srh.Groups.DecryptBox2(ctxt, srh.KeyPair.Id, msg.Previous())
	r.NoError(err)
	t.Log(string(clear))

	postRef, err := srh.Groups.PublishPostTo(cloaked, "just a small test group!")
	r.NoError(err)
	t.Log("post", postRef.ShortRef())

	msg, err = srh.Get(*postRef)
	r.NoError(err)
	content = msg.ContentBytes()
	r.True(bytes.HasSuffix(content, suffix), "%q", content)

	tal, err := sbot.New(
		sbot.WithContext(ctx),
		sbot.WithInfo(log.With(srvLog, "peer", "tal")),
		sbot.WithRepoPath(filepath.Join(testRepo, "tal")),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
		sbot.LateOption(sbot.MountSimpleIndex("get", indexes.OpenGet)), // todo muxrpc plugin is hardcoded
		sbot.LateOption(sbot.MountPlugin(&tangles.Plugin{}, plugins2.AuthMaster)),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(tal))

	_, err = tal.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "shalom!"})
	r.NoError(err)
	tal.PublishLog.Publish(refs.NewContactFollow(srh.KeyPair.Id))

	addMsgRef, err := srh.Groups.AddMember(cloaked, tal.KeyPair.Id, "welcome, tal!")
	r.NoError(err)
	t.Log("added:", addMsgRef.ShortRef())

	msg, err = srh.Get(*addMsgRef)
	r.NoError(err)
	content = msg.ContentBytes()
	r.True(bytes.HasSuffix(content, suffix), "%q", content)

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

	getSeq := func(l margaret.Log) int64 {
		sv, err := l.Seq().Value()
		r.NoError(err)

		seq, ok := sv.(margaret.Seq)
		r.True(ok, "wrong seq type: %T", sv)

		return seq.Seq()
	}

	r.EqualValues(1, getSeq(srhsCopyOfTal))
	r.EqualValues(3, getSeq(talsCopyOfSrh))

	reply, err := tal.Groups.PublishPostTo(cloaked, "thanks sarah!")
	r.NoError(err, "failed to publish to group")
	t.Log("reply:", reply.ShortRef())

	tal.Shutdown()
	srh.Shutdown()
	// cle.Shutdown()

	r.NoError(tal.Close())
	r.NoError(srh.Close())
	// r.NoError(cle.Close())
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
