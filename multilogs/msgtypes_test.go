package multilogs

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"reflect"
	"sync"
	"testing"

	//"github.com/cryptix/go/logging/logtest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
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

	ctx, cancel := context.WithCancel(context.TODO())

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
	alicePublish, err := OpenPublishLog(tRootLog, mt, *alice)
	r.NoError(err)

	bob, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	/*
		bobPublish, err := OpenPublishLog(tRootLog, mt, *bob)
		r.NoError(err)

		claire, err := ssb.NewKeyPair(nil)
		r.NoError(err)
		clairePublish, err := OpenPublishLog(tRootLog, mt, *claire)
		r.NoError(err)

		debby, err := ssb.NewKeyPair(nil)
		r.NoError(err)
	*/

	// > create contacts
	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":      "contact",
			"contact":   alice.Id.Ref(),
			"following": true,
		},
		map[string]interface{}{
			"type":     "contact",
			"contact":  bob.Id.Ref(),
			"blocking": true,
		},
		map[string]interface{}{
			"type":  "about",
			"about": bob.Id.Ref(),
			"name":  "bob",
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := alicePublish.Append(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}
	// < create contacts

	// not followed
	contactsLog, err := mt.Get(librarian.Addr("contact"))
	r.NoError(err, "error getting contacts log")
	contactsSrc, err := contactsLog.Query()
	r.NoError(err, "error querying contacts log")

	var i int

	contactsSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		defer func() { i++ }()

		if i > 1 && !luigi.IsEOS(err) {
			if err != nil {
				return errors.Errorf("unexpected error %q, expected EOS", v)
			}
			return errors.Errorf("unexpected value %v, expected EOS", v)
		}

		v, err = tRootLog.Get(v.(margaret.Seq))
		if err != nil {
			return errors.Errorf("error getting message from root log %q", v)
		}

		m := make(map[string]interface{})

		err = json.Unmarshal(v.(message.StoredMessage).Raw, &m)
		if err != nil {
			return errors.Errorf("error decoding stored message %q", v)
		}

		if !reflect.DeepEqual(m["content"], tmsgs[i]) {
			return errors.Errorf("unexpected value %+v instead of %+v at i=%v", v, tmsgs[i], i)
		}

		return nil
	})

	err = luigi.Pump(ctx, contactsSink, contactsSrc)
	r.NoError(err, "error pumping")

	mt.Close()
	uf.Close()
	cancel()

	for err := range mergedErrors(mtErrc, ufErrc) {
		r.NoError(err, "from chan")
	}
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
