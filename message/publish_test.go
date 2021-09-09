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

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/asynctesting"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

func TestSignMessages(t *testing.T) {
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

	authorLog, err := userFeeds.Get(storedrefs.Feed(testAuthor.ID()))
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
		// TODO: weird flake
		// currSeq, err := authorLog.Seq().Value()
		// r.NoError(err, "failed to get log seq")
		// r.Equal(newSeq, currSeq, "append messages was not current message?")
	}

	r.EqualValues(2, authorLog.Seq(), "not empty")

	for i := 0; i < len(tmsgs); i++ {
		rootSeq, err := authorLog.Get(int64(i))
		r.NoError(err)
		storedV, err := rl.Get(rootSeq.(int64))
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
		// a.NotNil(storedMsg.Raw, "msg:%d - raw", i)
		// a.Contains(string(storedMsg.Raw), `"signature": "`)
		// a.Contains(string(storedMsg.Raw), fmt.Sprintf(`"sequence": %d`, i+1))
	}

	cancel()
	r.NoError(<-errc, "serveLog failed")
}
