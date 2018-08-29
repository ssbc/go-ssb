package repo

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path"

	"github.com/cryptix/go/logging"
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
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
	"go.cryptoscope.co/sbot/message"
)

var _ Interface = (*repo)(nil)

var check = logging.CheckFatal

// New creates a new repository value, it opens the keypair and database from basePath if it is already existing
func New(basePath string) (Interface, error) {
	r := &repo{basePath: basePath}

	r.ctx = context.TODO() // TODO: pass in from main() to bind to signal handling shutdown
	r.ctx, r.shutdown = context.WithCancel(r.ctx)

	var err error
	r.blobStore, err = r.getBlobStore()
	if err != nil {
		return nil, errors.Wrap(err, "error creating blob store")
	}

	r.keyPair, err = r.getKeyPair()
	if err != nil {
		return nil, errors.Wrap(err, "error reading KeyPair")
	}

	r.rootLog, err = r.getRootLog()
	if err != nil {
		return nil, errors.Wrap(err, "error opening log")
	}

	if err := r.getUserFeeds(); err != nil {
		return nil, errors.Wrap(err, "error opening gossip index")
	}

	return r, nil
}

type repo struct {
	ctx      context.Context
	shutdown func()
	basePath string

	blobStore sbot.BlobStore
	keyPair   *sbot.KeyPair
	rootLog   margaret.Log

	userKV    *badger.DB
	userFeeds multilog.MultiLog

	contactsKV  *badger.DB
	contactsIdx librarian.SeqSetterIndex
}

func (r repo) Close() error {
	r.shutdown()
	// FIXME: does shutdown block..?
	// would be good to get back some kind of _all done without a problem_
	// time.Sleep(1 * time.Second)
	if err := r.contactsKV.Close(); err != nil {
		return errors.Wrap(r.userKV.Close(), "repo: failed to close contactsKV")
	}
	return errors.Wrap(r.userKV.Close(), "repo: failed to close userKV")
}

func (r *repo) getPath(rel string) string {
	return path.Join(r.basePath, rel)
}

func (r *repo) getKeyPair() (*sbot.KeyPair, error) {
	if r.keyPair != nil {
		return r.keyPair, nil
	}

	var err error
	secPath := r.getPath("secret")
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
			Id:   sbot.FeedRef{ID: kp.Public[:], Algo: "ed25519"},
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
		log.Println("warning: save new keypair!")
	}

	return r.keyPair, nil
}

func (r *repo) getRootLog() (margaret.Log, error) {
	if r.rootLog != nil {
		return r.rootLog, nil
	}

	logFile, err := os.OpenFile(r.getPath("log"), os.O_CREATE|os.O_RDWR, 0600)
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

func (r *repo) getUserFeeds() error {
	var err error
	if r.userFeeds != nil {
		return nil
	}

	// badger + librarian as index
	opts := badger.DefaultOptions
	opts.Dir = r.getPath("userFeeds.badger")
	opts.ValueDir = opts.Dir // we have small values in this one r.getPath("gossipBadger.values")
	r.userKV, err = badger.Open(opts)
	if err != nil {
		return errors.Wrap(err, "db/idx: badger failed to open")
	}
	r.userFeeds = multibadger.New(r.userKV, msgpack.New(margaret.BaseSeq(0)))

	idxStateFile, err := os.OpenFile(r.getPath("userFeedsState.json"), os.O_CREATE|os.O_RDWR, 0700)
	if err != nil {
		return errors.Wrap(err, "error opening gossip state file")
	}

	userFeedSink := multilog.NewSink(idxStateFile, r.userFeeds, func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		msg, ok := value.(message.StoredMessage)
		if !ok {
			return errors.Errorf("error casting message. got type %T", value)
		}

		authorLog, err := mlog.Get(librarian.Addr(msg.Author.ID))
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = authorLog.Append(seq)
		if err != nil {
			return errors.Wrap(err, "error appending new author message")
		}
		//log.Printf("indexed %s:%d as %d", msg.Author.Ref(), msg.Sequence, sublogSeq)
		return nil
	})

	go func() {
		src, err := r.rootLog.Query(margaret.Live(true), margaret.SeqWrap(true), userFeedSink.QuerySpec())
		check(errors.Wrap(err, ""))

		err = luigi.Pump(r.ctx, userFeedSink, src)
		if err != context.Canceled {
			check(errors.Wrap(err, "userFeeds index pump failed"))
		}
	}()

	contactsOpts := badger.DefaultOptions
	contactsOpts.Dir = r.getPath("contacts.badger")
	contactsOpts.ValueDir = contactsOpts.Dir // we have small values in this one r.getPath("gossipBadger.values")
	r.contactsKV, err = badger.Open(contactsOpts)
	if err != nil {
		return errors.Wrap(err, "db/idx: badger failed to open")
	}

	r.contactsIdx = libbadger.NewIndex(r.contactsKV, 0)
	contactsUpdate := func(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
		msg := val.(message.StoredMessage)
		var dmsg message.DeserializedMessage
		err := json.Unmarshal(msg.Raw, &dmsg)
		if err != nil {
			return errors.Wrap(err, "db/idx contacts: first json unmarshal failed")
		}

		var c sbot.Contact
		err = json.Unmarshal(dmsg.Content, &c)
		if err != nil {
			if sbot.IsMessageUnusable(err) {
				return nil
			}
			log.Println("repo contact skip msg warning:", err)
			return nil
		}

		addr := append(dmsg.Author.ID, ':')
		addr = append(addr, c.Contact.ID...)
		switch {
		case c.Following:
			err = idx.Set(ctx, librarian.Addr(addr), 1)
		case c.Blocking:
			err = idx.Set(ctx, librarian.Addr(addr), 0)
		default:
			err = idx.Delete(ctx, librarian.Addr(addr))
		}
		if err != nil {
			return errors.Wrapf(err, "db/idx contacts: failed to update index. %+v", c)
		}

		return err
	}
	contactsSink := librarian.NewSinkIndex(contactsUpdate, r.contactsIdx)
	go func() {
		src, err := r.rootLog.Query(margaret.Live(true), margaret.SeqWrap(true), contactsSink.QuerySpec())
		check(err)

		err = luigi.Pump(r.ctx, contactsSink, src)
		if err != context.Canceled {
			check(errors.Wrap(err, "userFeeds index pump failed"))
		}
	}()
	return nil
}

func (r *repo) IsFollowing(fr *sbot.FeedRef) ([]*sbot.FeedRef, error) {
	var friends []*sbot.FeedRef
	err := r.contactsKV.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		prefix := append(fr.ID, ':')

		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			it := iter.Item()
			k := it.Key()
			c := sbot.FeedRef{
				Algo: "ed25519",
				ID:   k[33:],
			}
			v, err := it.Value()
			if err != nil {
				return errors.Wrap(err, "friends: counldnt get idx value")
			}
			if len(v) >= 1 && v[0] == '1' {
				friends = append(friends, &c)
			}
		}
		return nil
	})

	return friends, err
}

func (r *repo) UserFeeds() multilog.MultiLog {
	return r.userFeeds
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
