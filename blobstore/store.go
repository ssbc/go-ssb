package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb"
)

var (
	ErrNoSuchBlob = errors.New("no such blob")
)

func parseBlobRef(refStr string) (*ssb.BlobRef, error) {
	ref, err := ssb.ParseRef(refStr)
	if err != nil {
		return nil, err
	}

	br, ok := ref.(*ssb.BlobRef)

	if !ok {
		return nil, fmt.Errorf("ref is not a %T but a %T", br, ref)
	}

	return br, nil
}

func New(basePath string) (ssb.BlobStore, error) {
	err := os.MkdirAll(filepath.Join(basePath, "sha256"), 0700)
	if err != nil {
		return nil, errors.Wrap(err, "error making dir for hash sha256")
	}

	err = os.MkdirAll(filepath.Join(basePath, "tmp"), 0700)
	if err != nil {
		return nil, errors.Wrap(err, "error making tmp dir")
	}

	bs := &blobStore{
		basePath: basePath,
	}

	bs.sink, bs.bcast = luigi.NewBroadcast()

	return bs, nil
}

type blobStore struct {
	basePath string

	sink  luigi.Sink
	bcast luigi.Broadcast
}

func (store *blobStore) getPath(ref *ssb.BlobRef) (string, error) {
	if ref.Algo != "sha256" {
		return "", errors.Errorf("unknown hash algorithm %q", ref.Algo)
	}
	if len(ref.Hash) != 32 {
		return "", errors.Errorf("expected hash length 32, got %v", len(ref.Hash))
	}

	hexHash := hex.EncodeToString(ref.Hash)
	relPath := filepath.Join(ref.Algo, hexHash[:2], hexHash[2:])

	return filepath.Join(store.basePath, relPath), nil
}

func (store *blobStore) getHexDirPath(ref *ssb.BlobRef) (string, error) {
	if ref.Algo != "sha256" {
		return "", errors.Errorf("unknown hash algorithm %q", ref.Algo)
	}
	if len(ref.Hash) != 32 {
		return "", errors.Errorf("expected hash length 32, got %v", len(ref.Hash))
	}

	hexHash := hex.EncodeToString(ref.Hash)
	relPath := filepath.Join(ref.Algo, hexHash[:2])

	return filepath.Join(store.basePath, relPath), nil
}

var i = 0

func (store *blobStore) getTmpPath() string {
	i++
	relPath := fmt.Sprintf("%d-%d", time.Now().UnixNano(), i)
	return filepath.Join(store.basePath, "tmp", relPath)
}

func (store *blobStore) Get(ref *ssb.BlobRef) (io.Reader, error) {
	blobPath, err := store.getPath(ref)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting path for ref %q", ref)
	}

	f, err := os.Open(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoSuchBlob
		}
		return nil, errors.Wrap(err, "error opening blob file")
	}

	return f, nil
}

func (store *blobStore) Put(blob io.Reader) (*ssb.BlobRef, error) {
	tmpPath := store.getTmpPath()
	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, errors.Wrapf(err, "blobstore.Put: error creating tmp file at %q", tmpPath)
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), blob)
	if err != nil && !luigi.IsEOS(err) {
		return nil, errors.Wrap(err, "blobstore.Put: error copying")
	}

	ref := &ssb.BlobRef{
		Hash: h.Sum(nil),
		Algo: "sha256",
	}

	if err := f.Close(); err != nil {
		return nil, errors.Wrap(err, "blobstore.Put: error closing tmp file")
	}

	hexDirPath, err := store.getHexDirPath(ref)
	if err != nil {
		return nil, errors.Wrap(err, "blobstore.Put: error getting hex dir path")
	}

	err = os.MkdirAll(hexDirPath, 0700)
	if err != nil {
		// ignore errors that indicate that the directory already exists
		if !os.IsExist(err) {
			return nil, errors.Wrap(err, "blobstore.Put: error creating hex dir")
		}
	}

	finalPath, err := store.getPath(ref)
	if err != nil {
		return nil, errors.Wrap(err, "blobstore.Put: error getting final path")
	}

	err = os.Rename(tmpPath, finalPath)
	if err != nil {
		return nil, errors.Wrapf(err, "error moving blob from temp path %q to final path %q", tmpPath, finalPath)
	}

	err = store.sink.Pour(context.TODO(), ssb.BlobStoreNotification{
		Op:  ssb.BlobStoreOpPut,
		Ref: ref,
	})

	return ref, errors.Wrap(err, "blobstore.Put: error in notification handler")
}

func (store *blobStore) Delete(ref *ssb.BlobRef) error {
	p, err := store.getPath(ref)
	if err != nil {
		return errors.Wrap(err, "error getting blob path")
	}

	err = os.Remove(p)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNoSuchBlob
		}
		return errors.Wrap(err, "error removing file")
	}

	err = store.sink.Pour(context.TODO(), ssb.BlobStoreNotification{
		Op:  ssb.BlobStoreOpRm,
		Ref: ref,
	})

	return errors.Wrap(err, "error in delete notification handlers")
}

func (store *blobStore) List() luigi.Source {
	return &listSource{
		basePath: filepath.Join(store.basePath, "sha256"),
	}
}

func (store *blobStore) Size(ref *ssb.BlobRef) (int64, error) {
	blobPath, err := store.getPath(ref)
	if err != nil {
		return 0, errors.Wrapf(err, "error getting path")
	}

	fi, err := os.Stat(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrNoSuchBlob
		}

		return 0, errors.Wrap(err, "error getting file info")
	}

	return fi.Size(), nil

}

func (store *blobStore) Changes() luigi.Broadcast {
	return store.bcast
}
