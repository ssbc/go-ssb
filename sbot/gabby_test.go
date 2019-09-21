package sbot

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/multilogs"
)

func TestGabbySync(t *testing.T) {
	r := require.New(t)
	// a := assert.New(t)
	ctx := context.TODO()

	os.RemoveAll("testrun")

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	aliLog, _ := logtest.KitLogger("ali", t)
	ali, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithInfo(aliLog),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
		LateOption(MountMultiLog("byTypes", multilogs.OpenMessageTypes)))
	r.NoError(err)

	var aliErrc = make(chan error, 1)
	go func() {
		err := ali.Network.Serve(ctx)
		if err != nil {
			aliErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(aliErrc)
	}()

	// bob is the one with the other feed format
	bobsKey, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	bobsKey.Id.Algo = ssb.RefAlgoFeedGabby

	bobLog, _ := logtest.KitLogger("bob", t)
	bob, err := New(
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithKeyPair(bobsKey),
		WithInfo(bobLog),
		WithRepoPath(filepath.Join("testrun", t.Name(), "bob")),
		WithListenAddr(":0"),
		LateOption(MountMultiLog("byTypes", multilogs.OpenMessageTypes)))
	r.NoError(err)

	var bobErrc = make(chan error, 1)
	go func() {
		err := bob.Network.Serve(ctx)
		if err != nil {
			bobErrc <- errors.Wrap(err, "bob serve exited")
		}
		close(bobErrc)
	}()

	// be friends
	seq, err := ali.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   bob.KeyPair.Id,
	})
	r.NoError(err)
	r.Equal(margaret.BaseSeq(0), seq)

	seq, err = bob.PublishLog.Append(ssb.Contact{
		Type:      "contact",
		Following: true,
		Contact:   ali.KeyPair.Id,
	})
	r.NoError(err)

	for i := 0; i < 9; i++ {
		seq, err := bob.PublishLog.Append(map[string]interface{}{
			"test": i,
		})
		r.NoError(err)
		r.Equal(margaret.BaseSeq(i+1), seq)
	}

	// sanity, check bob has his shit together
	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	bobsOwnLog, err := uf.Get(bob.KeyPair.Id.StoredAddr())
	r.NoError(err)

	seqv, err := bobsOwnLog.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(9), seqv, "bob doesn't have his own log!")

	// dial
	err = bob.Network.Connect(ctx, ali.Network.GetListenAddr())
	r.NoError(err)

	// give time to sync
	time.Sleep(3 * time.Second)
	// be done
	ali.Network.GetConnTracker().CloseAll()

	// check that bobs messages got to ali
	auf, ok := ali.GetMultiLog("userFeeds")
	r.True(ok)
	bosLogAtAli, err := auf.Get(bob.KeyPair.Id.StoredAddr())
	r.NoError(err)

	seqv, err = bosLogAtAli.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(9), seqv)

	src, err := mutil.Indirect(ali.RootLog, bosLogAtAli).Query()
	r.NoError(err)
	for {
		v, err := src.Next(ctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			r.NoError(err)
		}
		msg, ok := v.(*multimsg.MultiMessage)
		r.True(ok, "Type: %T", v)
		// t.Log(msg)
		_, ok = msg.AsGabby()
		r.True(ok)
		// a.True(msg.Author.ProtoChain)
		// a.NotEmpty(msg.ProtoChain)
	}

	ali.Shutdown()
	bob.Shutdown()
	r.NoError(ali.Close())
	r.NoError(bob.Close())

	r.NoError(<-mergeErrorChans(aliErrc, bobErrc))
}
