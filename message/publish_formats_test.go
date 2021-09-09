// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package message

import (
	"context"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/asynctesting"
	"go.cryptoscope.co/ssb/internal/multicloser"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message/legacy"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

func TestFormatsSimple(t *testing.T) {

	type testCase struct {
		// feed format
		ff refs.RefAlgo
	}
	var testCases = []testCase{
		{refs.RefAlgoFeedSSB1},
		{refs.RefAlgoFeedGabby},
		{refs.RefAlgoFeedBendyButt},
	}

	ts := newPublishtestSession(t)

	for _, tc := range testCases {
		t.Run(string(tc.ff), ts.makeFormatTest(tc.ff))
	}

	if err := ts.logCloser.Close(); err != nil {
		t.Error(err)
	}
}

type publishTestSession struct {
	rxLog margaret.Log

	logCloser io.Closer

	userLogs    multilog.MultiLog
	indexUpdate librarian.SinkIndex
}

func newPublishtestSession(t *testing.T) publishTestSession {
	r := require.New(t)
	rpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(rpath)

	var mc multicloser.MultiCloser

	testRepo := repo.New(rpath)

	rxl, err := repo.OpenLog(testRepo)
	r.NoError(err, "failed to open receive log")
	mc.AddCloser(rxl.(io.Closer))

	r.Equal(int64(-1), rxl.Seq(), "not empty")

	userLogs, idxUpdate, err := repo.OpenStandaloneMultiLog(testRepo, "testUsers", multilogs.UserFeedsUpdate)
	r.NoError(err, "failed to get user feeds multilog")
	mc.AddCloser(userLogs)
	mc.AddCloser(idxUpdate)

	return publishTestSession{
		rxLog: rxl,

		logCloser: &mc,

		userLogs:    userLogs,
		indexUpdate: idxUpdate,
	}
}

func (ts publishTestSession) makeFormatTest(ff refs.RefAlgo) func(t *testing.T) {
	staticRand := rand.New(rand.NewSource(42))
	return func(t *testing.T) {
		r := require.New(t)
		a := assert.New(t)

		testAuthor, err := ssb.NewKeyPair(staticRand, ff)
		r.NoError(err)

		authorLog, err := ts.userLogs.Get(storedrefs.Feed(testAuthor.ID()))
		r.NoError(err)

		w, err := OpenPublishLog(ts.rxLog, ts.userLogs, testAuthor)
		r.NoError(err, "publish log didnt open")

		var tmsgs = []interface{}{
			map[string]interface{}{
				"type":  "about",
				"about": testAuthor.ID().String(),
				"name":  "test user",
			},
			map[string]interface{}{
				"type":      "contact",
				"contact":   "@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519",
				"following": true,
			},
			map[string]interface{}{
				"type": "text",
				"text": `# hello world!`,
			},
		}
		for i, msg := range tmsgs {
			mr, err := w.Publish(msg)
			r.NoError(err, "failed to pour test message %d", i)
			r.NotNil(mr)
			errc := asynctesting.ServeLog(context.TODO(), t.Name(), ts.rxLog, ts.indexUpdate, false)
			r.NoError(<-errc)

			r.EqualValues(i, authorLog.Seq(), "failed to ")
		}

		r.EqualValues(2, authorLog.Seq(), "not empty %s", ff)

		for i := 0; i < len(tmsgs); i++ {
			rootSeq, err := authorLog.Get(int64(i))
			r.NoError(err)
			storedV, err := ts.rxLog.Get(rootSeq.(int64))
			r.NoError(err)
			storedMsg, ok := storedV.(refs.Message)
			r.True(ok)
			t.Logf("msg:%d\n%s", i, storedMsg.ValueContentJSON())
			a.NotNil(storedMsg.Key(), "msg:%d - key", i)

			// previous is correctly set
			if i != 0 {
				a.NotNil(storedMsg.Previous(), "msg:%d - previous", i)
				// get previous message
				prevV, err := authorLog.Get(int64(i - 1))
				r.NoError(err)
				prevSeq, ok := prevV.(int64)
				r.True(ok, "got:%T", prevV)
				prevV, err = ts.rxLog.Get(prevSeq)
				r.NoError(err)
				prevMsg, ok := prevV.(refs.Message)
				r.True(ok, "got:%T", prevV)

				a.True(prevMsg.Key().Equal(*storedMsg.Previous()), "msg:%d - wrong previous", i)
			} else {
				a.Nil(storedMsg.Previous(), "msg:%d - previous", i)
			}

			a.Equal(int64(i+1), storedMsg.Seq(), "msg:%d - has incorrect sequence")

			// verifies
			mm, ok := storedV.(*multimsg.MultiMessage)
			r.True(ok, "wrong type: %T", storedV)
			switch ff {
			case refs.RefAlgoFeedSSB1:
				msg, ok := mm.AsLegacy()
				r.True(ok)

				_, _, err = legacy.Verify(msg.Raw_, nil)
				r.NoError(err)

			case refs.RefAlgoFeedGabby:
				g, ok := mm.AsGabby()
				r.True(ok)
				a.True(g.Verify(nil), "gabby failed to validate msg:%d", i)

			case refs.RefAlgoFeedBendyButt:
				mf, ok := mm.AsMetaFeed()
				r.True(ok)
				a.True(mf.Verify(nil))
			default:
				r.FailNow("unhandled feed format", "format:%s", ff)
			}
		}
	}
}
