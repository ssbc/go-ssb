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
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	mksbot "go.cryptoscope.co/ssb/sbot"

	// debug
	_ "net/http/pprof"
)

var (
	// flags
	flagPromisc bool
	listenAddr  string
	debugAddr   string
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

	logging.SetupLogging(nil)
	log = logging.Logger("sbot")

	var err error
	appKey, err = base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
	checkFatal(err)

	u, err := user.Current()
	checkFatal(err)

	flag.StringVar(&listenAddr, "l", ":8008", "address to listen on")
	flag.StringVar(&debugAddr, "dbg", "localhost:6078", "listen addr for metrics and pprof HTTP server")
	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to put the log and indexes")

	flag.Parse()
}

func main() {
	ctx := context.Background()
	ctx, shutdown := context.WithCancel(ctx)

	defer func() {
		if r := recover(); r != nil {
			logging.LogPanicWithStack(log, "main-panic", r)
		}
	}()

	startDebug()

	sbot, err := mksbot.New(
		mksbot.WithInfo(log),
		mksbot.WithContext(ctx),
		mksbot.WithAppKey(appKey),
		mksbot.WithEventMetrics(SystemEvents, RepoStats, SystemSummary),
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

		time.Sleep(5 * time.Second)
		os.Exit(0)
	}()
	logging.SetCloseChan(c)

	id := sbot.KeyPair.Id
	uf := sbot.UserFeeds
	gb := sbot.GraphBuilder

	feeds, err := uf.List()
	checkFatal(err)
	log.Log("event", "repo open", "feeds", len(feeds))
	SystemEvents.With("event", "openedRepo").Add(1)
	RepoStats.With("part", "feeds").Set(float64(len(feeds)))

	var followCnt, msgCount uint
	for _, author := range feeds {
		subLog, err := uf.Get(author)
		checkFatal(err)

		currSeq, err := subLog.Seq().Value()
		checkFatal(err)
		msgCount += uint(currSeq.(margaret.BaseSeq))

		authorRef := ssb.FeedRef{
			Algo: "ed25519",
			ID:   []byte(author),
		}
		f, err := gb.Follows(&authorRef)
		checkFatal(err)
		log.Log("info", "currSeq", "feed", authorRef.Ref(), "seq", currSeq, "follows", len(f))
		followCnt += uint(len(f))

	}
	RepoStats.With("part", "msgs").Set(float64(msgCount))
	RepoStats.With("part", "follows").Set(float64(followCnt))

	log.Log("event", "serving", "ID", id.Ref(), "addr", listenAddr)
	for {
		err = sbot.Node.Serve(ctx, muxrpc.Throttle(500, 0), HandlerWithLatency(muxrpcSummary))
		log.Log("event", "sbot node.Serve returned", "err", err)
		SystemEvents.With("event", "nodeServ exited").Add(1)
		time.Sleep(1 * time.Second)
	}
}
