package multilogs

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/repo"
)

func TestTangles(t *testing.T) {
	// > test boilerplate
	// TODO: abstract serving and error channel handling
	// Meta TODO: close handling and status of indexing
	r := require.New(t)
	//info, _ := logtest.KitLogger(t.Name(), t)

	tRepoPath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)

	tRepo := repo.New(tRepoPath)
	tRootLog, err := repo.OpenLog(tRepo)
	r.NoError(err)

	uf, _, serveUF, err := OpenUserFeeds(tRepo)
	r.NoError(err)
	ufErrc := serveLog(ctx, "user feeds", tRootLog, serveUF)

	mt, _, serve, err := OpenTangles(tRepo)
	r.NoError(err)
	mtErrc := serveLog(ctx, "message types", tRootLog, serve)

	alice, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	alicePublish, err := OpenPublishLog(tRootLog, uf, *alice)
	r.NoError(err)

	bob, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	bobPublish, err := OpenPublishLog(tRootLog, uf, *bob)
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

	checkTypes(t, "contact", []string{"alice1"}, tRootLog, mt)
	checkTypes(t, "post", []string{"alice2"}, tRootLog, mt)
	checkTypes(t, "about", []string{"alice3"}, tRootLog, mt)

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

	checkTypes(t, "contact", []string{"alice1", "bob1"}, tRootLog, mt)
	checkTypes(t, "about", []string{"alice3", "bob2"}, tRootLog, mt)
	checkTypes(t, "post", []string{"alice2", "bob3"}, tRootLog, mt)

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

	checkTypes(t, "contact", []string{"alice1", "bob1", "claire2"}, tRootLog, mt)
	checkTypes(t, "about", []string{"alice3", "bob2", "claire3"}, tRootLog, mt)
	checkTypes(t, "post", []string{"alice2", "bob3", "claire1"}, tRootLog, mt)

	mt.Close()
	uf.Close()
	cancel()

	for err := range mergedErrors(mtErrc, ufErrc) {
		r.NoError(err, "from chan")
	}
}
