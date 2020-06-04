// SPDX-License-Identifier: MIT

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

	// debug
	_ "net/http/pprof"

	"github.com/cryptix/go/logging"
	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/debug"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/plugins2"
	"go.cryptoscope.co/ssb/plugins2/bytype"
	"go.cryptoscope.co/ssb/plugins2/names"
	"go.cryptoscope.co/ssb/plugins2/tangles"
	"go.cryptoscope.co/ssb/repo"
	mksbot "go.cryptoscope.co/ssb/sbot"
)

var (
	// flags
	flagCleanup  bool
	flagReindex  bool
	flagFSCK     string
	flagRepair   bool
	flagFatBot   bool
	flagHops     uint
	flagEnAdv    bool
	flagEnDiscov bool
	flagPromisc  bool

	flagDecryptPrivate  bool
	flagDisableUNIXSock bool

	listenAddr string
	debugAddr  string
	repoDir    string
	dbgLogDir  string

	// helper
	log        logging.Interface
	checkFatal = logging.CheckFatal

	// juicy bits
	appKey  string
	hmacSec string
)

// Version and Build are set by ldflags
var (
	Version = "snapshot"
	Build   = ""

	flagPrintVersion bool
)

func checkAndLog(err error) {
	if err != nil {
		if err := logging.LogPanicWithStack(log, "checkAndLog", err); err != nil {
			panic(err)
		}
	}
}

func initFlags() {
	u, err := user.Current()
	checkFatal(err)

	flag.UintVar(&flagHops, "hops", 1, "how many hops to fetch (1: friends, 2:friends of friends)")
	flag.BoolVar(&flagPromisc, "promisc", false, "bypass graph auth and fetch remote's feed")

	flag.StringVar(&appKey, "shscap", "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=", "secret-handshake app-key (or capability)")
	flag.StringVar(&hmacSec, "hmac", "", "if set, sign with hmac hash of msg, instead of plain message object, using this key")

	flag.StringVar(&listenAddr, "l", ":8008", "address to listen on")
	flag.BoolVar(&flagEnAdv, "localadv", false, "enable sending local UDP brodcasts")
	flag.BoolVar(&flagEnDiscov, "localdiscov", false, "enable connecting to incomming UDP brodcasts")

	flag.BoolVar(&flagDecryptPrivate, "decryptprivate", false, "store which messages can be decrypted")
	flag.BoolVar(&flagDisableUNIXSock, "nounixsock", false, "disable the UNIX socket RPC interface")

	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to put the log and indexes")

	flag.StringVar(&debugAddr, "dbg", "localhost:6078", "listen addr for metrics and pprof HTTP server")
	flag.StringVar(&dbgLogDir, "dbgdir", "", "where to write debug output to")

	flag.BoolVar(&flagFatBot, "fatbot", false, "if set, sbot loads additional index plugins (bytype, get, tangles)")
	flag.BoolVar(&flagReindex, "reindex", false, "if set, sbot exits after having its indicies updated")

	flag.BoolVar(&flagCleanup, "cleanup", false, "remove blocked feeds")

	flag.StringVar(&flagFSCK, "fsck", "", "run a filesystem check on the repo (possible values: length, sequences)")
	flag.BoolVar(&flagRepair, "repair", false, "run repo healing if fsck fails")

	flag.BoolVar(&flagPrintVersion, "version", false, "print version number and build date")

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

func runSbot() error {
	initFlags()

	if flagPrintVersion {
		log.Log("version", Version, "build", Build)
		return nil
	}

	ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)
	defer func() {
		cancel()
		if r := recover(); r != nil {
			logging.LogPanicWithStack(log, "main-panic", r)
		}
	}()

	ak, err := base64.StdEncoding.DecodeString(appKey)
	if err != nil {
		return errors.Wrap(err, "application key")
	}

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
	}

	if !flagDisableUNIXSock {
		opts = append(opts, mksbot.LateOption(mksbot.WithUNIXSocket()))
	}

	if flagDecryptPrivate {
		// TODO: refactor into plugins2
		r := repo.New(repoDir)
		kpsByPath, err := repo.AllKeyPairs(r)
		if err != nil {
			return errors.Wrap(err, "sbot: failed to open all keypairs in repo")
		}

		var kps []*ssb.KeyPair
		for _, v := range kpsByPath {
			kps = append(kps, v)
		}

		defKP, err := repo.DefaultKeyPair(r)
		if err != nil {
			return errors.Wrap(err, "sbot: failed to open default keypair")
		}
		kps = append(kps, defKP)

		mlogPriv := multilogs.NewPrivateRead(kitlog.With(log, "module", "privLogs"), kps...)

		opts = append(opts, mksbot.LateOption(mksbot.MountMultiLog("privLogs", mlogPriv.OpenRoaring)))
	}

	if flagFatBot {
		opts = append(opts,
			mksbot.LateOption(mksbot.MountSimpleIndex("get", indexes.OpenGet)), // todo muxrpc plugin is hardcoded
			mksbot.LateOption(mksbot.MountPlugin(&tangles.Plugin{}, plugins2.AuthMaster)),
			mksbot.LateOption(mksbot.MountPlugin(&names.Plugin{}, plugins2.AuthMaster)),
			mksbot.LateOption(mksbot.MountPlugin(&bytype.Plugin{}, plugins2.AuthMaster)),
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
		if err != nil {
			return errors.Wrap(err, "HMAC")
		}
		opts = append(opts, mksbot.WithHMACSigning(hcbytes))
	}

	if flagReindex {
		opts = append(opts,
			mksbot.DisableNetworkNode(),
			mksbot.DisableLiveIndexMode())
	}

	sbot, err := mksbot.New(opts...)
	if err != nil {
		return errors.Wrap(err, "scuttlebot")
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		level.Warn(log).Log("event", "killed", "msg", "received signal, shutting down", "signal", sig.String())
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
	uf, ok := sbot.GetMultiLog(multilogs.IndexNameFeeds)
	if !ok {
		checkAndLog(fmt.Errorf("missing userFeeds"))
		return nil
	}

	var fsckMode = mksbot.FSCKModeLength
	var exitAfterFSCK = false
	if flagFSCK != "" {
		switch flagFSCK {
		case "sequences":
			fsckMode = mksbot.FSCKModeSequences
		case "length":
			fsckMode = mksbot.FSCKModeLength
		default:
			return fmt.Errorf("unknown fsck mode: %q", flagFSCK)
		}
		exitAfterFSCK = true
	}

	err = sbot.FSCK(mksbot.FSCKWithFeedIndex(uf), mksbot.FSCKWithMode(fsckMode))
	if err != nil {
		if !flagRepair {
			return err
		}

		if report, ok := err.(mksbot.ErrConsistencyProblems); ok {
			err = sbot.HealRepo(report)
			if err != nil {
				level.Error(log).Log("fsck", "heal failed", "err", err)
			} else {
				level.Info(log).Log("fsck", "healed", "msgs", report.Sequences.GetCardinality(), "feeds", len(report.Errors))
			}
		} else {
			level.Error(log).Log("fsck", "wrong report type", "T", fmt.Sprintf("%T", err))

		}

		sbot.Shutdown()
		err := sbot.Close()
		checkAndLog(err)
		return nil
	}
	if exitAfterFSCK {
		level.Info(log).Log("fsck", "completed", "mode", fsckMode)
		sbot.Shutdown()
		err := sbot.Close()
		checkAndLog(err)
		return nil
	}

	SystemEvents.With("event", "openedRepo").Add(1)
	feeds, err := uf.List()
	if err != nil {
		return errors.Wrap(err, "user feed")
	}
	RepoStats.With("part", "feeds").Set(float64(len(feeds)))

	rseq, err := sbot.RootLog.Seq().Value()
	if err != nil {
		return errors.Wrap(err, "could not get root log sequence number")
	}
	msgCount := rseq.(margaret.Seq)
	RepoStats.With("part", "msgs").Set(float64(msgCount.Seq()))

	level.Info(log).Log("event", "repo open", "feeds", len(feeds), "msgs", msgCount)

	if flagReindex {
		level.Warn(log).Log("mode", "reindexing")
		err = sbot.FSCK(mksbot.FSCKWithMode(mksbot.FSCKModeSequences))
		if err != nil {
			return err
		}
		level.Warn(log).Log("mode", "fsck done")
		err = sbot.Close()
		checkAndLog(err)
		return nil
	}

	// removes blocked feeds
	if flagCleanup {
		level.Warn(log).Log("mode", "cleanup")

		tg, err := sbot.GraphBuilder.Build()
		if err != nil {
			return errors.Wrap(err, "failed to build graph during cleanup")
		}

		botRef := sbot.KeyPair.Id
		lst, err := tg.BlockedList(botRef).List()
		if err != nil {
			return errors.Wrap(err, "cleanup: failed to get blocked list")
		}

		for _, blocked := range lst {
			isStored, err := multilog.Has(uf, blocked.StoredAddr())
			if err != nil {
				return errors.Wrap(err, "blocked lookup in multilog")
			}

			if isStored {
				level.Info(log).Log("event", "nulled feed", "ref", blocked.Ref())
				err = sbot.NullFeed(blocked)
				if err != nil {
					return errors.Wrapf(err, "failed to null blocked feed %s", blocked.Ref())
				}
			}
		}

		sbot.Shutdown()
		return sbot.Close()
	}

	level.Info(log).Log("event", "serving", "ID", id.Ref(), "addr", listenAddr, "version", Version, "build", Build)
	for {
		// Note: This is where the serving starts ;)
		err = sbot.Network.Serve(ctx, HandlerWithLatency(muxrpcSummary))
		if err != nil {
			level.Warn(log).Log("event", "sbot node.Serve returned", "err", err)
		}
		SystemEvents.With("event", "nodeServ exited").Add(1)
		time.Sleep(1 * time.Second)
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
}

func main() {
	if err := runSbot(); err != nil {
		fmt.Fprintf(os.Stderr, "go-sbot: %s\n", err)
		os.Exit(1)
	}
}
