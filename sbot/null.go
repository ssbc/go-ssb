package sbot

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
)

// NullFeed overwrites all the entries from ref in repo with zeros
func NullFeed(r repo.Interface, ref *ssb.FeedRef) error {
	ctx := context.Background()

	uf, _, err := multilogs.OpenUserFeeds(r)
	if err != nil {
		err = errors.Wrap(err, "NullFeed: failed to open multilog")
		return err
	}
	defer uf.Close()

	rootLog, err := repo.OpenLog(r)
	if err != nil {
		err = errors.Wrap(err, "NullFeed: root-log open failed")
		return err
	}
	if c, ok := rootLog.(io.Closer); ok {
		defer c.Close()
	}

	alterLog, ok := rootLog.(margaret.Alterer)
	if !ok {
		err = errors.Errorf("NullFeed: not an alterer: %T", rootLog)
		return err
	}

	userSeqs, err := uf.Get(ref.StoredAddr())
	if err != nil {
		err = errors.Wrap(err, "NullFeed: failed to open log for feed argument")
		return err
	}

	src, err := userSeqs.Query() //margaret.SeqWrap(true))
	if err != nil {
		err = errors.Wrap(err, "NullFeed: failed create user seqs query")
		return err
	}

	i := 0
	snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		defer func() { i++ }()
		if err != nil {
			return err
		}
		seq, ok := v.(margaret.Seq)
		if !ok {
			return errors.Errorf("")
		}
		return alterLog.Null(seq)
	})
	err = luigi.Pump(ctx, snk, src)
	if err != nil {
		err = errors.Wrapf(err, "failed to pump entries and null them %d", i)
		return err
	}
	log.Printf("\ndropped %d entries", i)
	return nil
}

// Drop indicies deletes the following folders of the indexes.
// TODO: check that sbot isn't running?
func DropIndicies(r repo.Interface) error {

	// drop indicies
	var mlogs = []string{
		multilogs.IndexNameFeeds,
		multilogs.IndexNameTypes,
		multilogs.IndexNamePrivates,
	}
	for _, i := range mlogs {
		dbPath := r.GetPath(repo.PrefixMultiLog, i)
		err := os.RemoveAll(dbPath)
		if err != nil {
			err = errors.Wrapf(err, "mkdir error for %q", dbPath)
			return err
		}
	}
	var badger = []string{
		indexes.FolderNameContacts,
	}
	for _, i := range badger {
		dbPath := r.GetPath(repo.PrefixIndex, i)
		err := os.RemoveAll(dbPath)
		if err != nil {
			err = errors.Wrapf(err, "mkdir error for %q", dbPath)
			return err
		}
	}
	log.Println("removed index folders")
	return nil
}

func RebuildIndicies(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		err = errors.Wrap(err, "RebuildIndicies: failed to open sbot")
		return err
	}

	if !fi.IsDir() {
		return errors.Errorf("RebuildIndicies: repo path is not a directory")
	}

	// rebuilding indexes
	sbot, err := New(
		DisableNetworkNode(),
		WithRepoPath(path),
		DisableLiveIndexMode(),
	)
	if err != nil {
		err = errors.Wrap(err, "failed to open sbot")
		return err
	}

	// TODO: not sure if I should hook this signal here..
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		sbot.Shutdown()

		time.Sleep(5 * time.Second)

		err := sbot.Close()
		log.Println("sbot closed:", err)

		time.Sleep(5 * time.Second)
		os.Exit(0)
	}()

	start := time.Now()
	log.Println("started sbot for re-indexing")
	err = sbot.Close()
	log.Println("re-indexing took:", time.Since(start))
	return errors.Wrap(err, "RebuildIndicies: failed to close sbot")
}
