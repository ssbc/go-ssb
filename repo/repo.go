package repo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	idxbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/framing/lengthprefixed"
	"go.cryptoscope.co/margaret/offset"
	"go.cryptoscope.co/secretstream/secrethandshake"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
	"go.cryptoscope.co/sbot/message"
)

var _ sbot.Repo = (*repo)(nil)

func New(basePath string) (sbot.Repo, error) {
	r := &repo{basePath: basePath}

	var err error

	r.blobStore, err = r.getBlobStore()
	if err != nil {
		return nil, errors.Wrap(err, "error creating blob store")
	}

	r.keyPair, err = r.getKeyPair()
	if err != nil {
		return nil, errors.Wrap(err, "error reading KeyPair")
	}

	r.log, err = r.getLog()
	if err != nil {
		return nil, errors.Wrap(err, "error opening log")
	}

	if err := r.initGossipIndex(); err != nil {
		return nil, errors.Wrap(err, "error opening gossip index")
	}

	return r, nil
}

func (r *repo) FeedSeqs(fr sbot.FeedRef) ([]margaret.Seq, error) {
	var seqs []margaret.Seq
	err := r.gossipKv.View(func(txn *badger.Txn) error {
		itr := txn.NewIterator(badger.DefaultIteratorOptions)
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "%s:", fr.Ref())
		prefix := buf.Bytes()
		for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
			item := itr.Item()
			//k := item.Key()
			v, err := item.Value()
			if err != nil {
				return errors.Wrap(err, "failed to do value copy?!")
			}

			var seq margaret.Seq
			err = json.Unmarshal(v, &seq)
			// todo dynamic marshaller
			if err != nil {
				return errors.Wrap(err, "error unmarshaling using json marshaler")
			}
			seqs = append(seqs, seq)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to iterate prefixes")
	}
	return seqs, nil
}

func (r *repo) KnownFeeds() (map[string]margaret.Seq, error) {
	m := make(map[string]margaret.Seq)
	err := r.gossipKv.View(func(txn *badger.Txn) error {
		itr := txn.NewIterator(badger.DefaultIteratorOptions)
		prefix := []byte("latest:")
		for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
			item := itr.Item()
			k := item.Key()
			v, err := item.Value()
			if err != nil {
				return errors.Wrap(err, "failed to do value copy?!")
			}

			var seq margaret.Seq
			err = json.Unmarshal(v, &seq)
			// todo dynamic marshaller
			if err != nil {
				return errors.Wrap(err, "error unmarshaling using json marshaler")
			}

			m[string(bytes.TrimPrefix(k, prefix))] = seq
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to iterate prefixes")
	}
	return m, nil
}

type repo struct {
	basePath string

	blobStore sbot.BlobStore
	keyPair   *sbot.KeyPair
	log       margaret.Log
	gossipIdx librarian.SeqSetterIndex
	gossipKv  *badger.DB
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

func (r *repo) getLog() (margaret.Log, error) {
	if r.log != nil {
		return r.log, nil
	}

	logFile, err := os.OpenFile(r.getPath("log"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "error opening log file")
	}

	// TODO use proper log message type here
	r.log, err = offset.New(logFile, lengthprefixed.New32(16*1024), msgpack.New(&message.StoredMessage{}))
	return r.log, errors.Wrap(err, "failed to create log")
}

func (r *repo) initGossipIndex() error {
	var err error
	if r.gossipIdx != nil {
		return nil
	}

	// badger + librarian as index
	opts := badger.DefaultOptions
	opts.Dir = r.getPath("gossipBadger.keys")
	opts.ValueDir = opts.Dir // we have small values in this one r.getPath("gossipBadger.values")
	r.gossipKv, err = badger.Open(opts)
	if err != nil {
		return errors.Wrap(err, "db/idx: badger failed to open")
	}
	r.gossipIdx = idxbadger.NewIndex(r.gossipKv, margaret.Seq(-2))
	return nil
}

func (r *repo) GossipIndex() librarian.SeqSetterIndex {
	return r.gossipIdx
}

func (r *repo) Log() margaret.Log {
	return r.log
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
