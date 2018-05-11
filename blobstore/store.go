package blobstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"cryptoscope.co/go/sbot"
	"github.com/pkg/errors"
)

func New(basePath string) (sbot.BlobStore, error) {
	err := os.MkdirAll(path.Join(basePath, "sha256"), 0700)
	if err != nil {
		return nil, errors.Wrap(err, "error making dir for hash sha256")
	}

	err = os.MkdirAll(path.Join(basePath, "tmp"), 0700)
	if err != nil {
		return nil, errors.Wrap(err, "error making tmp dir")
	}

	return &blobStore{
		basePath: basePath,
	}, nil
}

type blobStore struct {
	basePath string
}

func (store *blobStore) getPath(ref *sbot.BlobRef) (string, error) {
	if ref.Algo != "sha256" {
		return "", errors.Errorf("unknown hash algorithm %q", ref.Algo)
	}
	if len(ref.Hash) != 32 {
		return "", errors.Errorf("expected hash length 32, got %v", len(ref.Hash))
	}

	hexHash := hex.EncodeToString(ref.Hash)
	relPath := path.Join(ref.Algo, hexHash[:2], hexHash[2:])

	return path.Join(store.basePath, relPath), nil
}

func (store *blobStore) getTmpPath() string {
	relPath := fmt.Sprint(time.Now().UnixNano())
	return path.Join(store.basePath, "tmp", relPath)
}

func (store *blobStore) Get(ref *sbot.BlobRef) (io.Reader, error) {
	blobPath, err := store.getPath(ref)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting path for ref %q", ref)
	}

	return os.Open(blobPath)
}

func (store *blobStore) Put(blob io.Reader) (*sbot.BlobRef, error) {
	tmpPath := store.getTmpPath()

	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating tmp file at %q", tmpPath)
	}

	h := sha256.New()
	tee := io.TeeReader(blob, h)
	_, err = io.Copy(f, tee)
	if err != nil {
		return nil, errors.Wrap(err, "error copying")
	}

	ref := &sbot.BlobRef{
		Hash: h.Sum(nil),
		Algo: "sha256",
	}

	finalPath, err := store.getPath(ref)
	if err != nil {
		return nil, errors.Wrap(err, "error getting final path")
	}

	err = os.Rename(tmpPath, finalPath)
	if err != nil {
		return nil, errors.Wrapf(err, "error moving blob from temp path %q to final path %q", tmpPath, finalPath)
	}

	return ref, nil
}
