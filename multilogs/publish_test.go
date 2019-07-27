package multilogs

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/keks/marx"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

func makeIndexServeWorker(name string, serveFunc repo.ServeFunc, rl margaret.Log, live bool) marx.Worker {
	return marx.Worker(func(ctx context.Context) error {
		err := serveFunc(ctx, rl, true)
		if err != nil && errors.Cause(err) != context.Canceled {
			return err
		}

		return nil
	})
}

func TestPublishUserFeeds(t *testing.T) {
	tctx, rootCancel := context.WithCancel(context.TODO())

	r := require.New(t)
	a := assert.New(t)

	unionErrChan := make(chan error)
	defer func() {
		rootCancel()
		err := <-unionErrChan
		r.NoError(err, "union error channel")

	}()

	unionWorker, joinTheUnion := marx.NewUnion()
	go func() {
		unionErrChan <- unionWorker(tctx)
	}()

	rpath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	testRepo := repo.New(rpath)

	_, err = repo.OpenKeyPair(testRepo)
	r.NoError(err, "failed to open key pair")

	rl, err := repo.OpenLog(testRepo)
	r.NoError(err, "failed to open root log")
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq, "not empty")

	userFeeds, _, userFeedsServe, err := OpenUserFeeds(testRepo)
	r.NoError(err, "failed to get user feeds multilog")
	joinTheUnion(makeIndexServeWorker("user feeds", userFeedsServe, rl, true))

	toplevel, _, toplevelServe, err := OpenToplevel(testRepo, 32)
	r.NoError(err, "failed to get toplevel multilog")
	joinTheUnion(makeIndexServeWorker("toplevel", toplevelServe, rl, true))

	staticRand := rand.New(rand.NewSource(42))
	testAuthor, err := ssb.NewKeyPair(staticRand)
	r.NoError(err)

	authorLog, err := userFeeds.Get(librarian.Addr(testAuthor.Id.ID))
	r.NoError(err)

	w, err := OpenPublishLog(rl, userFeeds, *testAuthor)
	r.NoError(err)

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": testAuthor.Id.Ref(),
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
		currSeq, err := authorLog.Seq().Value()
		r.NoError(err, "failed to get log seq")
		r.Equal(margaret.BaseSeq(i), newSeq, "advanced")
		r.Equal(newSeq, currSeq, "same new sequences")
	}

	latest, err := authorLog.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(2), latest, "not empty")

	for i := 0; i < len(tmsgs); i++ {
		rootSeq, err := authorLog.Get(margaret.BaseSeq(i))
		r.NoError(err)
		storedV, err := rl.Get(rootSeq.(margaret.Seq))
		r.NoError(err)
		storedMsg, ok := storedV.(message.StoredMessage)
		r.True(ok)
		t.Logf("msg:%d\n%q", i, storedMsg.Raw)
		a.NotNil(storedMsg.Key, "msg:%d - key", i)
		if i != 0 {
			a.NotNil(storedMsg.Previous, "msg:%d - previous", i)
		} else {
			a.Nil(storedMsg.Previous)
		}
		a.NotNil(storedMsg.Raw, "msg:%d - raw", i)
		a.Contains(string(storedMsg.Raw), `"signature": "`)
		a.Contains(string(storedMsg.Raw), fmt.Sprintf(`"sequence": %d`, i+1))
	}

	mustAddr := func(addr librarian.Addr, err error) librarian.Addr {
		r.NoError(err, "encodeStringTuple error")
		return addr
	}

	var toplevelExpect = map[librarian.Addr][]margaret.Seq{
		mustAddr(encodeStringTuple("type", "about")): []margaret.Seq{margaret.BaseSeq(0)},
		mustAddr(encodeStringTuple("type", "contact")): []margaret.Seq{margaret.BaseSeq(1)},
		mustAddr(encodeStringTuple("name", "test user")): []margaret.Seq{margaret.BaseSeq(0)},
		mustAddr(encodeStringTuple("type", "text")): []margaret.Seq{margaret.BaseSeq(2)},
		mustAddr(encodeStringTuple("text", "# hello world!")): []margaret.Seq{margaret.BaseSeq(2)},
	}

	for addr, seqs := range toplevelExpect {
		slog, err := toplevel.Get(addr)
		r.NoErrorf(err, "open toplevel sublog %q", addr)

		ctx, cancel := context.WithCancel(tctx)

		sink := luigi.FuncSink(func(ctx_ context.Context, v interface{}, err error) error{
			r.True(len(seqs) > 0)
			r.Equal(seqs[0], v)

			seqs = seqs[1:]

			if len(seqs) == 0 {
				go func(addr librarian.Addr) {
					cancel()
				}(addr)
			}

			return nil
		})

		src, err := slog.Query(margaret.Live(true))
		r.NoErrorf(err, "query toplevel sublog %q", addr)

		err = luigi.Pump(ctx, sink, src)
		r.EqualError(errors.Cause(err), "context canceled", "luigi pump error")

		a.Equalf(len(seqs), 0, "pair %q not complete", addr)
	}

}

