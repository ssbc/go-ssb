package repo

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/cryptix/go/logging"
	"github.com/dgraph-io/badger"
	kitlog "github.com/go-kit/kit/log"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/framing/lengthprefixed"
	"go.cryptoscope.co/margaret/multilog"
	multibadger "go.cryptoscope.co/margaret/multilog/badger"
	"go.cryptoscope.co/margaret/offset"
	"go.cryptoscope.co/secretstream/secrethandshake"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
	"go.cryptoscope.co/sbot/graph"
	"go.cryptoscope.co/sbot/message"
)

var _ Interface = (*repo)(nil)

var check = logging.CheckFatal

// New creates a new repository value, it opens the keypair and database from basePath if it is already existing
func New(log logging.Interface, basePath string, opts ...Option) (Interface, error) {
	r := &repo{basePath: basePath, log: log}

	for i, o := range opts {
		err := o(r)
		if err != nil {
			return nil, errors.Wrapf(err, "repo: failed to apply option %d", i)
		}
	}

	if r.ctx == nil {
		r.ctx = context.Background()
	}
	r.ctx, r.shutdown = context.WithCancel(r.ctx)

	var err error
	r.blobStore, err = r.getBlobStore()
	if err != nil {
		return nil, errors.Wrap(err, "error creating blob store")
	}

	if r.keyPair == nil {
		r.keyPair, err = r.getKeyPair()
		if err != nil {
			return nil, errors.Wrap(err, "error reading KeyPair")
		}
	}

	r.rootLog, err = r.getRootLog()
	if err != nil {
		return nil, errors.Wrap(err, "error opening log")
	}

	contactsPath := r.GetPath("indexes", "contacts", "db")
	err = os.MkdirAll(contactsPath, 0700)
	if err != nil {
		return nil, errors.Wrap(err, "error making contact module directory")
	}

	r.graphBuilder, err = graph.NewBuilder(kitlog.With(log, "module", "graph"), contactsPath)
	if err != nil {
		return nil, errors.Wrap(err, "error opening contact graph builder")
	}

	go func() {
		src, err := r.rootLog.Query(margaret.Live(true), margaret.SeqWrap(true), r.graphBuilder.QuerySpec())
		check(err)

		err = luigi.Pump(r.ctx, r.graphBuilder, src)
		if err != context.Canceled {
			check(errors.Wrap(err, "contacts index pump failed"))
		}
	}()

	return r, nil
}

type repo struct {
	ctx      context.Context
	log      logging.Interface
	shutdown func()
	basePath string

	blobStore sbot.BlobStore
	keyPair   *sbot.KeyPair
	rootLog   margaret.Log

	graphBuilder graph.Builder
}

func (r repo) Close() error {
	r.shutdown()
	// FIXME: does shutdown block..?
	// would be good to get back some kind of _all done without a problem_
	// time.Sleep(1 * time.Second)

	var err error

	if e := r.graphBuilder.Close(); err != nil {
		err = multierror.Append(err, errors.Wrap(e, "repo: failed to close graph builder"))
	}

	return err
}

func (r *repo) GetPath(rel ...string) string {
	return path.Join(append([]string{r.basePath}, rel...)...)
}

func (r *repo) getKeyPair() (*sbot.KeyPair, error) {
	if r.keyPair != nil {
		return r.keyPair, nil
	}

	var err error
	secPath := r.GetPath("secret")
	r.keyPair, err = sbot.LoadKeyPair(secPath)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			return nil, errors.Wrap(err, "error opening key pair")
		}
		// generating new keypair
		kp, err := secrethandshake.GenEdKeyPair(nil)
		if err != nil {
			return nil, errors.Wrap(err, "error building key pair")
		}
		r.keyPair = &sbot.KeyPair{
			Id:   &sbot.FeedRef{ID: kp.Public[:], Algo: "ed25519"},
			Pair: *kp,
		}
		// TODO:
		// keyFile, err := os.Create(secPath)
		// if err != nil {
		// 	return nil, errors.Wrap(err, "error creating secret file")
		// }
		// if err:=sbot.SaveKeyPair(keyFile);err != nil {
		// 	return nil, errors.Wrap(err, "error saving secret file")
		// }
		fmt.Println("warning: save new keypair!")
	}

	return r.keyPair, nil
}

func (r *repo) getRootLog() (margaret.Log, error) {
	if r.rootLog != nil {
		return r.rootLog, nil
	}

	logFile, err := os.OpenFile(r.GetPath("log"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "error opening log file")
	}

	// TODO use proper log message type here
	// FIXME: 16kB because some messages are even larger than 12kB - even though the limit is supposed to be 8kb
	r.rootLog, err = offset.New(logFile, lengthprefixed.New32(16*1024), msgpack.New(&message.StoredMessage{}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create rootLog")
	}

	return r.rootLog, nil
}

func (r *repo) RootLog() margaret.Log {
	return r.rootLog
}

func (r *repo) KeyPair() sbot.KeyPair {
	return *r.keyPair
}

func (r *repo) Plugins() []sbot.Plugin {
	return nil
}

func (r *repo) Builder() graph.Builder {
	return r.graphBuilder
}

func (r *repo) getBlobStore() (sbot.BlobStore, error) {
	if r.blobStore != nil {
		return r.blobStore, nil
	}

	bs, err := blobstore.New(path.Join(r.basePath, "blobs"))
	if err != nil {
		return nil, errors.Wrap(err, "error creating blob store")
	}

	r.blobStore = bs
	return bs, nil
}

func (r *repo) BlobStore() sbot.BlobStore {
	return r.blobStore
}

// GetMultiLog uses the repo to determine the paths where to finds the multilog with given name and opens it.
//
// Exposes the badger db for 100% hackability. This will go away in future versions!
func GetMultiLog(r Interface, name string, f multilog.Func) (multilog.MultiLog, *badger.DB, func(context.Context, margaret.Log) error, error) {
	// badger + librarian as index
	opts := badger.DefaultOptions

	dbPath := r.GetPath("sublogs", name, "db")
	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "mkdir error for %q", dbPath)
	}

	opts.Dir = dbPath
	opts.ValueDir = opts.Dir // we have small values in this one

	db, err := badger.Open(opts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "db/idx: badger failed to open")
	}

	mlog := multibadger.New(db, msgpack.New(margaret.BaseSeq(0)))

	statePath := r.GetPath("sublogs", name, "state.json")
	idxStateFile, err := os.OpenFile(statePath, os.O_CREATE|os.O_RDWR, 0700)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error opening state file")
	}

	mlogSink := multilog.NewSink(idxStateFile, mlog, f)

	serve := func(ctx context.Context, rootLog margaret.Log) error {
		src, err := rootLog.Query(margaret.Live(true), margaret.SeqWrap(true), mlogSink.QuerySpec())
		if err != nil {
			return errors.Wrap(err, "error querying rootLog for mlog")
		}

		err = luigi.Pump(ctx, mlogSink, src)
		if err == context.Canceled {
			return nil
		}

		return errors.Wrap(err, "error reading query for mlog")
	}

	return mlog, db, serve, nil
}
