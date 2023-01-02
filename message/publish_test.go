// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package message

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ssbc/go-ssb"
	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb/internal/asynctesting"
	"github.com/ssbc/go-ssb/internal/testutils"
	"github.com/ssbc/go-ssb/multilogs"
	"github.com/ssbc/go-ssb/repo"
)

func TestSignMessages(t *testing.T) {
	if testutils.SkipOnCI(t) {
		// https://github.com/ssbc/go-ssb/pull/170
		return
	}
	
	tctx := context.TODO()
	r := require.New(t)
	a := assert.New(t)

	rpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(rpath)

	testRepo := repo.New(rpath)
	rl, err := repo.OpenLog(testRepo)
	t.Cleanup(func() {
		rl.Close()
	})

	r.NoError(err, "failed to open root log")
	r.EqualValues(-1, rl.Seq(), "not empty")

	userFeeds, userFeedsSnk, err := repo.OpenStandaloneMultiLog(testRepo, "testUsers", multilogs.UserFeedsUpdate)
	r.NoError(err, "failed to get user feeds multilog")
	t.Cleanup(func() {
		userFeeds.Close()
		userFeedsSnk.Close()
	})

	killServe, cancel := context.WithCancel(tctx)
	defer cancel()
	errc := asynctesting.ServeLog(killServe, t.Name(), rl, userFeedsSnk, true)

	staticRand := rand.New(rand.NewSource(42))
	testAuthor, err := ssb.NewKeyPair(staticRand, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	w, err := OpenPublishLog(rl, userFeeds, testAuthor)
	r.NoError(err)

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
		newSeq, err := w.Append(msg)
		r.NoError(err, "failed to pour test message %d", i)
		r.EqualValues(i, newSeq, "advanced")
		currSeq := rl.Seq()
		r.NoError(err, "failed to get log seq")
		r.Equal(newSeq, currSeq, "append messages was not current message?")
	}

	for i := 0; i < len(tmsgs); i++ {
		storedV, err := rl.Get(int64(i))
		r.NoError(err)
		storedMsg, ok := storedV.(refs.Message)
		r.True(ok)
		t.Logf("msg:%d\n%s", i, storedMsg.ContentBytes())
		a.NotNil(storedMsg.Key(), "msg:%d - key", i)
		if i != 0 {
			a.NotNil(storedMsg.Previous(), "msg:%d - previous", i)
		} else {
			a.Nil(storedMsg.Previous(), "msg:%d - expected nil previous", i)
		}
		a.NotNil(storedMsg.ContentBytes(), "msg:%d - raw", i)
		value := storedMsg.ValueContent()
		a.NotNil(value.Signature, "msg:%d - expected signature", i)
		a.NotNil(value.Sequence, "msg:%d - expected sequence number", i)
	}

	cancel()
	r.NoError(<-errc, "serveLog failed")
}
