package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/sbot"
)

var (
	ErrNoSuchBlob = errors.New("no such blob")
)

func parseBlobRef(refStr string) (*sbot.BlobRef, error) {
	ref, err := sbot.ParseRef(refStr)
	if err != nil {
		return nil, err
	}

	br, ok := ref.(*sbot.BlobRef)

	if !ok {
		return nil, fmt.Errorf("ref is not a %T but a %T", br, ref)
	}

	return br, nil
}

func New(basePath string) (sbot.BlobStore, error) {
	err := os.MkdirAll(path.Join(basePath, "sha256"), 0700)
	if err != nil {
		return nil, errors.Wrap(err, "error making dir for hash sha256")
	}

	err = os.MkdirAll(path.Join(basePath, "tmp"), 0700)
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

func (store *blobStore) getHexDirPath(ref *sbot.BlobRef) (string, error) {
	if ref.Algo != "sha256" {
		return "", errors.Errorf("unknown hash algorithm %q", ref.Algo)
	}
	if len(ref.Hash) != 32 {
		return "", errors.Errorf("expected hash length 32, got %v", len(ref.Hash))
	}

	hexHash := hex.EncodeToString(ref.Hash)
	relPath := path.Join(ref.Algo, hexHash[:2])

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

	f, err := os.Open(blobPath)
	if err != nil {
		perr, ok := err.(*os.PathError)
		if ok && perr.Err == syscall.ENOENT {
			return nil, ErrNoSuchBlob
		}

		return nil, errors.Wrap(err, "error opening blob file")
	}

	return f, nil
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

	hexDirPath, err := store.getHexDirPath(ref)
	if err != nil {
		return nil, errors.Wrap(err, "error getting hex dir path")
	}

	err = os.Mkdir(hexDirPath, 0700)
	if err != nil {
		perr, ok := err.(*os.PathError)
		// ignore errors that indicate that the directory already exists
		if !ok || perr.Err != syscall.EEXIST {
			return nil, errors.Wrap(err, "error creating hex dir")
		}
	}

	finalPath, err := store.getPath(ref)
	if err != nil {
		return nil, errors.Wrap(err, "error getting final path")
	}

	err = os.Rename(tmpPath, finalPath)
	if err != nil {
		return nil, errors.Wrapf(err, "error moving blob from temp path %q to final path %q", tmpPath, finalPath)
	}

	err = store.sink.Pour(context.TODO(), sbot.BlobStoreNotification{
		Op:  sbot.BlobStoreOpPut,
		Ref: ref,
	})

	return ref, errors.Wrap(err, "error in notification handler")
}

func (store *blobStore) Delete(ref *sbot.BlobRef) error {
	p, err := store.getPath(ref)
	if err != nil {
		return errors.Wrap(err, "error getting blob path")
	}

	err = os.Remove(p)
	if err != nil {
		perr, ok := err.(*os.PathError)
		if ok && perr.Err == syscall.ENOENT {
			return ErrNoSuchBlob
		}

		return errors.Wrap(err, "error removing file")
	}

	err = store.sink.Pour(context.TODO(), sbot.BlobStoreNotification{
		Op:  sbot.BlobStoreOpRm,
		Ref: ref,
	})

	return errors.Wrap(err, "error in delete notification handlers")
}

func (store *blobStore) List() luigi.Source {
	return &listSource{
		basePath: path.Join(store.basePath, "sha256"),
	}
}

func (store *blobStore) Size(ref *sbot.BlobRef) (int64, error) {
	blobPath, err := store.getPath(ref)
	if err != nil {
		return 0, errors.Wrapf(err, "error getting path")
	}

	fi, err := os.Stat(blobPath)
	if err != nil {
		perr, ok := err.(*os.PathError)
		if ok && perr.Err == syscall.ENOENT {
			return 0, ErrNoSuchBlob
		}

		return 0, errors.Wrap(err, "error getting file info")
	}

	return fi.Size(), nil

}

func (store *blobStore) Changes() luigi.Broadcast {
	return store.bcast
}
