// SPDX-License-Identifier: MIT

// +build ignore

// TODO: mutli-author refactor

package tangles

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/repo"
	"go.mindeco.de/log"
)

func TestTangles(t *testing.T) {
	r := require.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(tRepoPath)

	// make three new keypairs with nicknames
	n2kp := make(map[string]*ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", ssb.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kpCloe, err := repo.NewKeyPair(tRepo, "cloe", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["cloe"] = kpCloe

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 3)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	mainbot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		WithHMACSigning(hk),
		LateOption(MountSimpleIndex("get", indexes.OpenGet)),
		DisableNetworkNode(),
	)
	r.NoError(err)

	// > create contacts
	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":      "contact",
			"contact":   alice.Id.Ref(),
			"following": true,
			"test":      "alice1",
		},
		map[string]interface{}{
			"type": "post",
			"text": "something",
			"test": "alice2",
		},
		"1923u1310310.nobox",
		map[string]interface{}{
			"type":  "about",
			"about": bob.Id.Ref(),
			"name":  "bob",
			"test":  "alice3",
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := alicePublish.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	var bobsMsgs = []interface{}{
		map[string]interface{}{
			"type":      "contact",
			"contact":   bob.Id.Ref(),
			"following": true,
			"test":      "bob1",
		},
		"1923u1310310.nobox",
		map[string]interface{}{
			"type":  "about",
			"about": bob.Id.Ref(),
			"name":  "bob",
			"test":  "bob2",
		},
		map[string]interface{}{
			"type": "post",
			"text": "hello, world",
			"test": "bob3",
		},
	}
	for i, msg := range bobsMsgs {
		newSeq, err := bobPublish.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	var clairesMsgs = []interface{}{
		"1923u1310310.nobox",
		map[string]interface{}{
			"type": "post",
			"text": "hello, world",
			"test": "claire1",
		},
		map[string]interface{}{
			"type":      "contact",
			"contact":   claire.Id.Ref(),
			"following": true,
			"test":      "claire2",
		},
		map[string]interface{}{
			"type":  "about",
			"about": claire.Id.Ref(),
			"name":  "claire",
			"test":  "claire3",
		},
	}
	for i, msg := range clairesMsgs {
		newSeq, err := clairePublish.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	t.Fail()
	/* TODO: update to same roots
	checkTypes(t, "contact", []string{"alice1", "bob1", "claire2"}, tRootLog, mt)
	checkTypes(t, "about", []string{"alice3", "bob2", "claire3"}, tRootLog, mt)
	checkTypes(t, "post", []string{"alice2", "bob3", "claire1"}, tRootLog, mt)
	*/

	mt.Close()
	uf.Close()
	cancel()

	for err := range mergedErrors(mtErrc, ufErrc) {
		r.NoError(err, "from chan")
	}
}
