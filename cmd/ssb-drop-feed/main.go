// ssb-drop-feed nulls entries of one particular feed from repo
// there is no warning or undo
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
	mksbot "go.cryptoscope.co/ssb/sbot"
)

func check(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	fmt.Fprintln(os.Stderr, "occurred at")
	debug.PrintStack()
	os.Exit(1)
}

func main() {
	ctx := context.TODO()
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <repo> <@feed=...>\n", os.Args[0])
		os.Exit(1)
	}

	r := repo.New(os.Args[1])
	fr, err := ssb.ParseFeedRef(os.Args[2])
	check(errors.Wrap(err, "failed to parse feed argument"))

	uf, _, _, err := multilogs.OpenUserFeeds(r)
	check(errors.Wrap(err, "failed to open multilog"))
	defer uf.Close()

	rootLog, err := repo.OpenLog(r)
	check(errors.Wrap(err, "root-log open failed"))
	if c, ok := rootLog.(io.Closer); ok {
		defer c.Close()
	}

	alterLog, ok := rootLog.(margaret.Alterer)
	if !ok {
		check(errors.Errorf("not an alterer: %T", rootLog))
	}

	userSeqs, err := uf.Get(librarian.Addr(fr.ID))
	check(errors.Wrap(err, "failed to open log for feed argument"))

	src, err := userSeqs.Query() //margaret.SeqWrap(true))
	check(errors.Wrap(err, "failed create user seqs query"))

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
		fmt.Printf("\rdropped %d", seq.Seq())
		return alterLog.Null(seq)
	})
	err = luigi.Pump(ctx, snk, src)
	check(errors.Wrapf(err, "failed to pump entries and null them %d", i))

	log.Printf("\ndropped %d entries", i)

	// dropp indicies
	var mlogs = []string{
		multilogs.IndexNameFeeds,
		multilogs.IndexNameTypes,
		multilogs.IndexNamePrivates,
	}
	for _, i := range mlogs {
		dbPath := r.GetPath(repo.PrefixMultiLog, i)
		err := os.RemoveAll(dbPath)
		check(errors.Wrapf(err, "mkdir error for %q", dbPath))
	}
	var badger = []string{
		indexes.FolderNameAbout,
		indexes.FolderNameContacts,
	}
	for _, i := range badger {
		dbPath := r.GetPath(repo.PrefixIndex, i)
		err := os.RemoveAll(dbPath)
		check(errors.Wrapf(err, "mkdir error for %q", dbPath))
	}

	fmt.Println("removed index folders")

	// rebuilding indexes
	// ctx, done := ctxutils.WithError(ctx, ssb.ErrShuttingDown)
	sbot, err := mksbot.New(
		mksbot.WithContext(ctx),
		mksbot.DisableNetworkNode(),
		mksbot.WithListenAddr("127.0.0.1:0"),
		mksbot.WithRepoPath(os.Args[1]),
		mksbot.DisableLiveIndexMode(),
	)
	check(errors.Wrap(err, "failed to open sbot"))

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c

		sbot.Shutdown()

		time.Sleep(2 * time.Second)

		err := sbot.Close()
		check(err)

		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()
	logging.SetCloseChan(c)

	fmt.Println("re-started sbot for indexing")
	check(sbot.Close())
	fmt.Println("done")
}
