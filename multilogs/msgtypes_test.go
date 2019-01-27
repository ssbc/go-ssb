package multilogs

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
)

type mlogInitFunc func(ctx context.Context, r repo.Interface, log margaret.Log, newSink newErrorSinkFunc) (uf multilog.MultiLog, mt multilog.MultiLog, err error)

func directMlogInit(ctx context.Context, r repo.Interface, log margaret.Log, newSink newErrorSinkFunc) (uf multilog.MultiLog, mt multilog.MultiLog, err error) {
	var serveUF, serveMT func(context.Context, margaret.Log) error

	uf, _, serveUF, err = OpenUserFeeds(r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error opening user feeds multilog")
	}

	mt, _, serveMT, err = OpenMessageTypes(r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error opening message types multilog")
	}

	go func() {
		newSink() <- errors.Wrap(serveUF(ctx, log), "user feeds: serve exited")
	}()

	go func() {
		newSink() <- errors.Wrap(serveMT(ctx, log), "message types: serve exited")
	}()

	return uf, mt, nil
}

func managedMlogInit(ctx context.Context, r repo.Interface, log margaret.Log, newSink newErrorSinkFunc) (uf multilog.MultiLog, mt multilog.MultiLog, err error) {
	mgr, err := NewManager(r, log)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error initializing multilog manager")
	}

	go func() {
		newSink() <- errors.Wrap(mgr.Serve(ctx), "user feeds: serve exited")
	}()

	return mgr.UserFeeds(), mgr.MessageTypes(), nil
}

func TestMessageTypes(t *testing.T) {
	t.Run("mlog-direct", mkTestMessageTypes(directMlogInit))
	t.Run("mlog-managed", mkTestMessageTypes(managedMlogInit))
}

func mkTestMessageTypes(initMlog mlogInitFunc) func(*testing.T) {
	return func(t *testing.T) {
		// > test boilerplate
		// Meta TODO: close handling and status of indexing
		r := require.New(t)
		//info, _ := logtest.KitLogger(t.Name(), t)

		errSrc, mkErrSink := newErrorCollector()

		tRepoPath, err := ioutil.TempDir("", strings.Replace(t.Name(), "/", "_", -1))
		r.NoError(err)

		ctx, cancel := context.WithCancel(context.TODO())

		tRepo := repo.New(tRepoPath)
		tRootLog, err := repo.OpenLog(tRepo)
		r.NoError(err)

		uf, mt, err := initMlog(ctx, tRepo, tRootLog, mkErrSink)
		r.NoError(err)
		r.NotNil(uf, "user feeds multilog is nil")
		r.NotNil(mt, "message types multilog is nil")

		alice, err := ssb.NewKeyPair(nil)
		r.NoError(err)

		alicePublish, err := OpenPublishLog(tRootLog, mt, *alice)
		r.NoError(err)

		bob, err := ssb.NewKeyPair(nil)
		r.NoError(err)
		bobPublish, err := OpenPublishLog(tRootLog, mt, *bob)
		r.NoError(err)

		claire, err := ssb.NewKeyPair(nil)
		r.NoError(err)
		clairePublish, err := OpenPublishLog(tRootLog, mt, *claire)
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

		for err := range errSrc {
			r.NoError(err, "from chan")
		}
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

		err = json.Unmarshal(v.(message.StoredMessage).Raw, &m)
		if err != nil {
			return errors.Errorf("error decoding stored message %q", v)
		}
		mt := m["content"].(map[string]interface{})
		if got := mt["test"]; got != texpected[i] {
			return errors.Errorf("unexpected value %+v instead of %+v at i=%v", got, texpected[i], i)
		}

		return nil
	})

	return snk, closeC
}

type margaretServe func(context.Context, margaret.Log) error

func serveLog(ctx context.Context, name string, l margaret.Log, f margaretServe) <-chan error {
	errc := make(chan error)
	go func() {
		err := f(ctx, l)
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
