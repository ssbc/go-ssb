package multilogs

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/asynctesting"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

func TestMessageTypes(t *testing.T) {
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

	uf, serveUF, err := OpenUserFeeds(tRepo)
	r.NoError(err)
	ufErrc := asynctesting.ServeLog(ctx, "user feeds", tRootLog, serveUF)

	mt, serveMT, err := OpenMessageTypes(tRepo)
	r.NoError(err)
	mtErrc := asynctesting.ServeLog(ctx, "message types", tRootLog, serveMT)

	alice, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	alicePublish, err := message.OpenPublishLog(tRootLog, mt, alice)
	r.NoError(err)

	bob, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	bobPublish, err := message.OpenPublishLog(tRootLog, mt, bob)
	r.NoError(err)

	claire, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	clairePublish, err := message.OpenPublishLog(tRootLog, mt, claire)
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

	asynctesting.CheckTypes(t, "contact", []string{"alice1"}, tRootLog, mt)
	asynctesting.CheckTypes(t, "post", []string{"alice2"}, tRootLog, mt)
	asynctesting.CheckTypes(t, "about", []string{"alice3"}, tRootLog, mt)

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

	asynctesting.CheckTypes(t, "contact", []string{"alice1", "bob1"}, tRootLog, mt)
	asynctesting.CheckTypes(t, "about", []string{"alice3", "bob2"}, tRootLog, mt)
	asynctesting.CheckTypes(t, "post", []string{"alice2", "bob3"}, tRootLog, mt)

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

	asynctesting.CheckTypes(t, "contact", []string{"alice1", "bob1", "claire2"}, tRootLog, mt)
	asynctesting.CheckTypes(t, "about", []string{"alice3", "bob2", "claire3"}, tRootLog, mt)
	asynctesting.CheckTypes(t, "post", []string{"alice2", "bob3", "claire1"}, tRootLog, mt)

	mt.Close()
	uf.Close()
	cancel()

	for err := range asynctesting.MergedErrors(mtErrc, ufErrc) {
		r.NoError(err, "from chan")
	}
}
