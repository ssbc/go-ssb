package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
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
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/plugins2"
	"go.cryptoscope.co/ssb/plugins2/tangles"
	mksbot "go.cryptoscope.co/ssb/sbot"

	// debug
	_ "net/http/pprof"
)

var (
	// flags
	flagsReindex bool
	flagFatBot   bool
	flagHops     uint
	flagEnAdv    bool
	flagEnDiscov bool
	flagPromisc  bool
	listenAddr   string
	debugAddr    string
	repoDir      string
	dbgLogDir    string

	// helper
	log        logging.Interface
	checkFatal = logging.CheckFatal

	// juicy bits
	appKey  string
	hmacSec string
)

func checkAndLog(err error) {
	if err != nil {
		if err := logging.LogPanicWithStack(log, "checkAndLog", err); err != nil {
			panic(err)
		}
	}
}

func init() {
	u, err := user.Current()
	checkFatal(err)

	flag.UintVar(&flagHops, "hops", 1, "how many hops to fetch (1: friends, 2:friends of friends)")
	flag.BoolVar(&flagPromisc, "promisc", false, "bypass graph auth and fetch remote's feed")

	flag.StringVar(&appKey, "shscap", "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=", "secret-handshake app-key (or capability)")
	flag.StringVar(&hmacSec, "hmac", "", "if set, sign with hmac hash of msg, instead of plain message object, using this key")

	flag.StringVar(&listenAddr, "l", ":8008", "address to listen on")
	flag.BoolVar(&flagEnAdv, "localadv", false, "enable sending local UDP brodcasts")
	flag.BoolVar(&flagEnDiscov, "localdiscov", false, "enable connecting to incomming UDP brodcasts")

	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to put the log and indexes")

	flag.StringVar(&debugAddr, "dbg", "localhost:6078", "listen addr for metrics and pprof HTTP server")
	flag.StringVar(&dbgLogDir, "dbgdir", "", "where to write debug output to")

	flag.BoolVar(&flagFatBot, "fatbot", false, "if set, sbot loads additional index plugins (bytype, get, tangles)")
	flag.BoolVar(&flagsReindex, "reindex", false, "if set, sbot exits after having its indicies updated")

	flag.Parse()

	if dbgLogDir != "" {
		logDir := filepath.Join(repoDir, dbgLogDir)
		os.MkdirAll(logDir, 0700) // nearly everything is a log here so..
		logFileName := fmt.Sprintf("%s-%s.log",
			filepath.Base(os.Args[0]),
			time.Now().Format("2006-01-02_15-04"))
		logFile, err := os.Create(filepath.Join(logDir, logFileName))
		if err != nil {
			panic(err) // logging not ready yet...
		}
		logging.SetupLogging(logFile)
	} else {
		logging.SetupLogging(os.Stderr)
	}
	log = logging.Logger("sbot")
}

func main() {
	ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)
	defer func() {
		cancel()
		if r := recover(); r != nil {
			logging.LogPanicWithStack(log, "main-panic", r)
		}
	}()

	ak, err := base64.StdEncoding.DecodeString(appKey)
	checkFatal(err)

	startDebug()
	opts := []mksbot.Option{
		mksbot.WithHops(flagHops),
		mksbot.WithPromisc(flagPromisc),
		mksbot.WithInfo(log),
		mksbot.WithAppKey(ak),
		mksbot.WithRepoPath(repoDir),
		mksbot.WithListenAddr(listenAddr),
		mksbot.EnableAdvertismentBroadcasts(flagEnAdv),
		mksbot.EnableAdvertismentDialing(flagEnDiscov),
		mksbot.WithUNIXSocket(),
	}

	if flagFatBot {
		opts = append(opts,
			mksbot.LateOption(mksbot.MountMultiLog("byTypes", multilogs.OpenMessageTypes)),
			// mksbot.LateOption(mksbot.MountSimpleIndex("get", indexes.OpenGet)),
			mksbot.LateOption(mksbot.MountPlugin(&tangles.Plugin{}, plugins2.AuthMaster)),
		// TODO: make about
		)
	}

	if dbgLogDir != "" {
		opts = append(opts, mksbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
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
	}

	if debugAddr != "" {
		opts = append(opts,
			mksbot.WithEventMetrics(SystemEvents, RepoStats, SystemSummary),
			mksbot.WithPreSecureConnWrapper(promCountConn()),
		)
	}

	if hmacSec != "" {
		hcbytes, err := base64.StdEncoding.DecodeString(hmacSec)
		checkFatal(err)
		opts = append(opts, mksbot.WithHMACSigning(hcbytes))
	}

	if flagsReindex {
		opts = append(opts,
			mksbot.DisableNetworkNode(),
			mksbot.DisableLiveIndexMode())
	}

	sbot, err := mksbot.New(opts...)
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

	if flagsReindex {
		log.Log("mode", "reindexing")
		err = sbot.Close()
		checkAndLog(err)
		return
	}

	id := sbot.KeyPair.Id
	uf, ok := sbot.GetMultiLog("userFeeds")
	if !ok {
		checkAndLog(fmt.Errorf("missing userFeeds"))
		return
	}

	feeds, err := uf.List()
	checkFatal(err)

	SystemEvents.With("event", "openedRepo").Add(1)
	RepoStats.With("part", "feeds").Set(float64(len(feeds)))

	sbot.FSCK(uf)

	rseq, err := sbot.RootLog.Seq().Value()
	checkFatal(err)
	msgCount := rseq.(margaret.Seq)
	RepoStats.With("part", "msgs").Set(float64(msgCount.Seq()))

	log.Log("event", "repo open", "feeds", len(feeds), "msgs", msgCount)

	log.Log("event", "serving", "ID", id.Ref(), "addr", listenAddr)
	for {
		// Note: This is where the serving starts ;)
		err = sbot.Network.Serve(ctx, HandlerWithLatency(muxrpcSummary))
		log.Log("event", "sbot node.Serve returned", "err", err)
		SystemEvents.With("event", "nodeServ exited").Add(1)
		time.Sleep(1 * time.Second)
		select {
		case <-ctx.Done():
			os.Exit(0)
		default:
		}
	}
}
