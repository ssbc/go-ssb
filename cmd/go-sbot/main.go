package main

import (
	"context"
	"encoding/base64"
	"flag"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/debug"
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
	dbgLogDir   string

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
	flag.StringVar(&dbgLogDir, "dbgdir", "", "where to write debug output to")
	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to put the log and indexes")

	flag.Parse()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		if r := recover(); r != nil {
			logging.LogPanicWithStack(log, "main-panic", r)
		}
	}()

	startDebug()

	sbot, err := mksbot.New(
		// TODO: hops
		// TOOD: promisc
		mksbot.WithInfo(log),
		mksbot.WithAppKey(appKey),
		mksbot.WithEventMetrics(SystemEvents, RepoStats, SystemSummary),
		mksbot.WithRepoPath(repoDir),
		mksbot.WithListenAddr(listenAddr),
		mksbot.WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
			if dbgLogDir == "" {
				return conn, nil
			}

			parts := strings.Split(conn.RemoteAddr().String(), "|")

			if len(parts) != 2 {
				return conn, nil
			}

			muxrpcDumpDir := filepath.Join(
				repoDir,
				dbgLogDir,
				parts[1], // key first
				parts[0],
			)

			return debug.WrapDump(muxrpcDumpDir, conn)
		}))
	checkFatal(err)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Log("event", "killed", "msg", "received signal, shutting down", "signal", sig.String())
		cancel()
		sbot.Shutdown()
		time.Sleep(2 * time.Second)

		err := sbot.Close()
		checkAndLog(err)

		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()
	logging.SetCloseChan(c)

	id := sbot.KeyPair.Id
	uf := sbot.UserFeeds
	gb := sbot.GraphBuilder

	feeds, err := uf.List()
	checkFatal(err)

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
		if len(feeds) < 20 {
			h := gb.Hops(&authorRef, 2)
			log.Log("info", "currSeq", "feed", authorRef.Ref(), "seq", currSeq, "follows", f.Count(), "hops", h.Count())
		}
		followCnt += uint(f.Count())
	}

	RepoStats.With("part", "msgs").Set(float64(msgCount))
	// abouts, err := sbot.MessageTypes.Get("about")
	// checkFatal(err)
	// logSeqV, err := abouts.Seq().Value()
	// checkFatal(err)
	// RepoStats.With("part", "about").Set(float64(logSeqV.(margaret.Seq).Seq()))
	// contactLog, err := sbot.MessageTypes.Get("contact")
	// checkFatal(err)
	// logSeqV, err := contactLog.Seq().Value()
	// checkFatal(err)
	// RepoStats.With("part", "contact").Set(float64(logSeqV.(margaret.Seq).Seq()))
	log.Log("event", "repo open", "feeds", len(feeds), "msgs", msgCount, "follows", followCnt)

	log.Log("event", "serving", "ID", id.Ref(), "addr", listenAddr)
	for {
		err = sbot.Node.Serve(ctx, HandlerWithLatency(muxrpcSummary))
		log.Log("event", "sbot node.Serve returned", "err", err)
		SystemEvents.With("event", "nodeServ exited").Add(1)
		time.Sleep(1 * time.Second)
	}
}
