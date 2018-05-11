package repo

import (
	"os"
	"path"

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

	return r, nil
}

type repo struct {
	basePath string

	blobStore sbot.BlobStore
	keyPair   *secrethandshake.EdKeyPair
}

func (r *repo) getPath(rel string) string {
	return path.Join(r.basePath, rel)
}

func (r *repo) getKeyPair() (*secrethandshake.EdKeyPair, error) {
	if r.keyPair != nil {
		return r.keyPair, nil
	}

	keyFile, err := os.Open(r.getPath("secret"))
	if err != nil {
		return nil, errors.Wrap(err, "error opening secret file")
	}

	r.keyPair, err = secrethandshake.GenEdKeyPair(keyFile)
	if err != nil {
		return nil, errors.Wrap(err, "error building key pair")
	}

	return r.keyPair, nil
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
