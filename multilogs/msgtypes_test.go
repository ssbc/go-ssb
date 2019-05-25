package multilogs

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
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

	uf, _, serveUF, err := OpenUserFeeds(tRepo)
	r.NoError(err)
	ufErrc := serveLog(ctx, "user feeds", tRootLog, serveUF)

	mt, _, serveMT, err := OpenMessageTypes(tRepo)
	r.NoError(err)
	mtErrc := serveLog(ctx, "message types", tRootLog, serveMT)

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

func checkTypes(t *testing.T, tipe string, tExpected []string, rootLog margaret.Log, mt multilog.MultiLog) {
	r := require.New(t)

	typeLog, err := mt.Get(librarian.Addr(tipe))
	r.NoError(err, "error getting contacts log")
	typeLogQuery, err := typeLog.Query()
	r.NoError(err, "error querying contacts log")

	alice1Sink, wait := makeCompareSink(tExpected, rootLog)
	err = luigi.Pump(context.TODO(), alice1Sink, typeLogQuery)
	r.NoError(err, "error pumping")
	err = alice1Sink.Close()
	r.NoError(err, "error closing sink")
	<-wait
}

func makeCompareSink(texpected []string, rootLog margaret.Log) (luigi.FuncSink, chan struct{}) {
	var i int
	closeC := make(chan struct{})
	snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		defer func() { i++ }()
		var expectEOS = i > len(texpected)-1
		if err != nil {
			if luigi.IsEOS(err) {
				if expectEOS {
					close(closeC)
					return nil
				}
				return errors.Errorf("expected more values after err %v  i%d, len%d", err, i, len(texpected))
			}
			return errors.Errorf("unexpected error %q", err)
		}

		v, err = rootLog.Get(v.(margaret.Seq))
		if err != nil {
			return errors.Errorf("error getting message from root log %q", v)
		}
		if expectEOS {
			return errors.Errorf("expected EOS but got value(%d) %v", i, v)
		}

		m := make(map[string]interface{})

		abs := v.(ssb.Message)
		err = json.Unmarshal(abs.ContentBytes(), &m)
		if err != nil {
			return errors.Errorf("error decoding stored message %q", v)
		}
		if got := m["test"]; got != texpected[i] {
			return errors.Errorf("unexpected value %+v instead of %+v at i=%v", got, texpected[i], i)
		}

		return nil
	})

	return snk, closeC
}

func serveLog(ctx context.Context, name string, l margaret.Log, f repo.ServeFunc) <-chan error {
	errc := make(chan error)
	go func() {
		err := f(ctx, l, true)
		if err != nil {
			errc <- errors.Wrapf(err, "%s serve exited", name)
		}
		close(errc)
	}()
	return errc
}

func mergedErrors(cs ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error)

	output := func(c <-chan error) {
		for a := range c {
			out <- a
		}
		wg.Done()
	}

	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
