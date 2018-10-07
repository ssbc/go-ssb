package main

import (
	"context"
	"encoding/base64"
	"flag"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cryptix/go/logging"

	"go.cryptoscope.co/ssb"
	mksbot "go.cryptoscope.co/ssb/sbot"

	// debug
	"net/http"
	_ "net/http/pprof"
)

func startHTTPServer() {
	err := http.ListenAndServe("localhost:6078", nil)
	if err != nil {
		panic(err)
	}
}

var (
	// flags
	flagPromisc bool
	listenAddr  string
	repoDir     string

	// helper
	log        logging.Interface
	checkFatal = logging.CheckFatal

	// juicy bits
	appKey []byte
)

func checkAndLog(err error) {
	if err != nil {
		if err := logging.LogPanicWithStack(log, "checkAndLog", err); err != nil {
			panic(err)
		}
	}
}

func init() {
	go startHTTPServer() // debug

	logging.SetupLogging(nil)
	log = logging.Logger("sbot")

	var err error
	appKey, err = base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
	checkFatal(err)

	u, err := user.Current()
	checkFatal(err)

	flag.StringVar(&listenAddr, "l", ":8008", "address to listen on")
	flag.BoolVar(&flagPromisc, "promisc", false, "crawl all the feeds")
	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to put the log and indexes")

	flag.Parse()
}

func main() {
	ctx := context.Background()
	ctx, shutdown := context.WithCancel(ctx)

	sbot, err := mksbot.New(
		mksbot.WithInfo(log),
		mksbot.WithContext(ctx),
		mksbot.WithAppKey(appKey),
		mksbot.WithRepoPath(repoDir),
		mksbot.WithListenAddr(listenAddr))

	checkFatal(err)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Log("event", "killed", "msg", "received signal, shutting down", "signal", sig.String())
		shutdown()

		err := sbot.Close()
		checkAndLog(err)

		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	logging.SetCloseChan(c)

	id := sbot.KeyPair.Id
	uf := sbot.UserFeeds
	gb := sbot.GraphBuilder

	feeds, err := uf.List()
	checkFatal(err)
	log.Log("event", "repo open", "feeds", len(feeds))
	for _, author := range feeds {
		subLog, err := uf.Get(author)
		checkFatal(err)

		currSeq, err := subLog.Seq().Value()
		checkFatal(err)

		authorRef := ssb.FeedRef{
			Algo: "ed25519",
			ID:   []byte(author),
		}
		f, err := gb.Follows(&authorRef)
		checkFatal(err)

		log.Log("info", "currSeq", "feed", authorRef.Ref(), "seq", currSeq, "follows", len(f))
	}

	log.Log("event", "serving", "ID", id.Ref(), "addr", listenAddr)
	for {
		err = sbot.Node.Serve(ctx)
		log.Log("event", "sbot node.Serve returned", "err", err)
		time.Sleep(1 * time.Second)
	}
}
