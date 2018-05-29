package repo

import (
	"os"
	"path"

	"cryptoscope.co/go/margaret"
	mjson "cryptoscope.co/go/margaret/codec/json"
	"cryptoscope.co/go/margaret/framing/lengthprefixed"
	"cryptoscope.co/go/margaret/offset"
	"cryptoscope.co/go/secretstream/secrethandshake"
	"github.com/pkg/errors"

	"cryptoscope.co/go/sbot"
	"cryptoscope.co/go/sbot/blobstore"
)

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

	return r, nil
}

type repo struct {
	basePath string

	blobStore sbot.BlobStore
	keyPair   *secrethandshake.EdKeyPair
	log       margaret.Log
}

func (r *repo) getPath(rel string) string {
	return path.Join(r.basePath, rel)
}

func (r *repo) getKeyPair() (*secrethandshake.EdKeyPair, error) {
	if r.keyPair != nil {
		return r.keyPair, nil
	}

	var err error
	r.keyPair, err = secrethandshake.LoadSSBKeyPair(r.getPath("secret"))
	if err != nil {
		return nil, errors.Wrap(err, "error building key pair")
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
	r.log = offset.NewOffsetLog(logFile, lengthprefixed.New32(8*1024), mjson.New(nil))
	return r.log, nil
}

func (r *repo) Log() margaret.Log {
	return r.log
}

func (r *repo) KeyPair() secrethandshake.EdKeyPair {
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
