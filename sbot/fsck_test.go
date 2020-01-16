package sbot

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/repo"
)

func makeTestBot(t *testing.T, ctx context.Context) *Sbot {
	r := require.New(t)

	testPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(testPath)

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	mainLog := testutils.NewRelativeTimeLogger(nil)

	// some extra keypairs for multi-feed fun
	tRepo := repo.New(testPath)
	_, err := repo.NewKeyPair(tRepo, "one", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	_, err = repo.NewKeyPair(tRepo, "two", ssb.RefAlgoFeedGabby)
	r.NoError(err)

	botOptions := []Option{
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithContext(ctx),
		WithInfo(log.With(mainLog, "unit", "theBot")),
		WithRepoPath(testPath),
		DisableNetworkNode(),
	}
	theBot, err := New(botOptions...)
	r.NoError(err)
	return theBot
}

func TestFSCK(t *testing.T) {
	t.Run("correct", testFSCKcorrect)
	t.Run("duplicate", testFSCKduplicate)
	t.Run("multipleFeeds", testFSCKmultipleFeeds)
}

func testFSCKcorrect(t *testing.T) {
	r := require.New(t)
	ctx, cancel := context.WithCancel(context.TODO())
	theBot := makeTestBot(t, ctx)

	const n = 32
	for i := n; i > 0; i-- {
		_, err := theBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	err := theBot.FSCK(nil, FSCKModeLength)
	r.NoError(err)

	err = theBot.FSCK(nil, FSCKModeSequences)
	r.NoError(err)

	// cleanup
	theBot.Shutdown()
	cancel()
	r.NoError(theBot.Close())
}

func testFSCKduplicate(t *testing.T) {
	r := require.New(t)
	ctx, cancel := context.WithCancel(context.TODO())
	theBot := makeTestBot(t, ctx)

	const n = 32
	for i := n; i > 0; i-- {
		_, err := theBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	// now do some nasty magic, duplicate the log by appending it to itself again
	// TODO: refactor to only have Add() on the bot, not the internal rootlog
	// Add() should do the append logic
	src, err := theBot.RootLog.Query(margaret.Limit(n))
	r.NoError(err)

	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			r.NoError(err)
		}

		seq, err := theBot.RootLog.Append(v)
		r.NoError(err)
		t.Log("duplicated:", seq.Seq())
	}

	seqv, err := theBot.RootLog.Seq().Value()
	r.NoError(err)

	seq := seqv.(margaret.Seq)
	r.EqualValues(seq.Seq()+1, n*2)

	err = theBot.FSCK(nil, FSCKModeLength)
	r.Error(err)
	t.Log(err)

	err = theBot.FSCK(nil, FSCKModeSequences)
	r.Error(err)
	constErrs, ok := err.(ssb.ErrConsistencyProblems)
	r.True(ok, "wrong error type. got %T", err)
	r.Len(constErrs.Errors, 1)
	r.Contains(constErrs.Errors[0].Error(), "consistency error: message sequence missmatch")
	t.Log(err)

	// cleanup
	theBot.Shutdown()
	cancel()
	r.NoError(theBot.Close())
}

// TODO: copy a corrupted subset of the feed to a fresh rootlog, reindex and see the error

func testFSCKmultipleFeeds(t *testing.T) {
	r := require.New(t)
	ctx, cancel := context.WithCancel(context.TODO())
	theBot := makeTestBot(t, ctx)

	// some "correct" messages
	const n = 32
	for i := n; i > 0; i-- {
		_, err := theBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"one", map[string]interface{}{"hello": 123}},
		{"one", map[string]interface{}{"world": 456}},
		{"two", map[string]interface{}{"test": 123}},
		{"two", map[string]interface{}{"test": 456}},
		{"two", map[string]interface{}{"test": 789}},
	}
	for idx, intro := range intros {
		ref, err := theBot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
	}

	// copy the messages from one and two (leaving "main" intact)
	src, err := theBot.RootLog.Query(
		margaret.Gt(margaret.BaseSeq(n-1)),
		margaret.Limit(5))
	r.NoError(err)
	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			r.NoError(err)
		}

		msg, ok := v.(ssb.Message)
		r.True(ok)

		seq, err := theBot.RootLog.Append(v)
		r.NoError(err)
		t.Log("duplicated:", msg.Author().Ref()[1:5], seq.Seq())
	}

	err = theBot.FSCK(nil, FSCKModeLength)
	r.Error(err)
	t.Log(err)

	err = theBot.FSCK(nil, FSCKModeSequences)
	r.Error(err)
	constErrs, ok := err.(ssb.ErrConsistencyProblems)
	r.True(ok, "wrong error type. got %T", err)
	r.Len(constErrs.Errors, 2)

	// cleanup
	theBot.Shutdown()
	cancel()
	r.NoError(theBot.Close())
}
