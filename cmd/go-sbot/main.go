// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// go-sbot hosts the database and p2p server for replication.
// It supplies various flags to contol options.
// See 'go-sbot -h' for a list and their usage.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2/debug"
	"go.mindeco.de/log/level"
	"go.mindeco.de/logging"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/multilogs"
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

	flagEnableEBT bool

	flagDisableUNIXSock bool

	repoDir     string
	listenAddr  string
	wsLisAddr   string
	debugAddr   string
	debugLogDir string
	configPath   string

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
		level.Error(log).Log("event", "fatal error", "err", err)
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

	flag.StringVar(&listenAddr, "lis", ":8008", "address to listen on")
	flag.BoolVar(&flagEnAdv, "localadv", false, "enable sending local UDP brodcasts")
	flag.BoolVar(&flagEnDiscov, "localdiscov", false, "enable connecting to incomming UDP brodcasts")

	flag.StringVar(&wsLisAddr, "wslis", ":8989", "address to listen on for ssb-ws connections")

	flag.BoolVar(&flagEnableEBT, "enable-ebt", false, "enable syncing by using epidemic-broadcast-trees (new code, test with caution)")

	flag.BoolVar(&flagDisableUNIXSock, "nounixsock", false, "disable the UNIX socket RPC interface")

	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to put the log and indexes")

	flag.StringVar(&debugAddr, "debuglis", "localhost:6078", "listen addr for metrics and pprof HTTP server")
	flag.StringVar(&debugLogDir, "debugdir", "", "where to write debug output to")

	flag.StringVar(&configPath, "config", filepath.Join(u.HomeDir, ".ssb-go"), "path to folder containing go-ssb config file (if using)")

	flag.BoolVar(&flagReindex, "reindex", false, "if set, sbot exits after having its indicies updated")

	flag.BoolVar(&flagCleanup, "cleanup", false, "remove blocked feeds")

	flag.StringVar(&flagFSCK, "fsck", "", "run a filesystem check on the repo (possible values: length, sequences)")
	flag.BoolVar(&flagRepair, "repair", false, "run repo healing if fsck fails")

	flag.BoolVar(&flagPrintVersion, "version", false, "print version number and build date")

	flag.Parse()
}

func applyConfigValues() {
	/*
	 It's config & environment variable reading time! We read the config and/or any set environment variables first.
	 Then, for each flag that has NOT been set and which corresponds to a config/env value, we set the flag variable's
	 value to the value's found in conf / env variable.

	 The hierarchy goes as follows:
	 * flag set values trump environment variables
	 * environment variables trumps config values
	 * set config values trump default flag values
	 * default flag values are the final fallback, if the corresponding config value or environment variable has not been
	   set
	*/
	// returns true if the named flag was passed to go-sbot on startup
	isFlagPassed := func(name string) bool {
		found := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == name {
				found = true
			}
		})
		return found
	}

	if !strings.HasSuffix(configPath, "toml") {
		configPath = filepath.Join(configPath, "config.toml")
	}
	configDir := filepath.Dir(configPath)
	if val := os.Getenv("SSB_CONFIG_FILE"); val != "" {
		configPath = val
	}
	config := ReadConfigAndEnv(configPath)
	bconfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		panic(err)
	}
	runningConfigPath := filepath.Join(configDir, "running-config.json")
	err = os.WriteFile(runningConfigPath, bconfig, 0644)
	if err != nil {
		panic(err)
	} else {
		level.Info(log).Log("event", "write running-config.json", "msg", "active config and env vars have been persisted", "path", runningConfigPath)
	}
	// Returns true if the config has a value for flagname set, and the flag itself isn't passed on invocation
	UseConfigValue := func(flagname string) bool {
		return config.Has(flagname) && !isFlagPassed(flagname)
	}

	if UseConfigValue("hops") {
		flagHops = config.Hops
	}
	if UseConfigValue("promisc") {
		flagPromisc = (bool)(config.EnableFirewall)
	}
	if UseConfigValue("shscap") {
		appKey = config.ShsCap
	}
	if UseConfigValue("repo") {
		repoDir = config.Repo
	}
	if UseConfigValue("lis") {
		listenAddr = config.MuxRPCAddress
	}
	if UseConfigValue("localadv") {
		flagEnAdv = (bool)(config.EnableAdvertiseUDP)
	}
	if UseConfigValue("localdiscov") {
		flagEnDiscov = (bool)(config.EnableDiscoveryUDP)
	}
	if UseConfigValue("wslis") {
		wsLisAddr = config.WebsocketAddress
	}
	if UseConfigValue("enable-ebt") {
		flagEnableEBT = (bool)(config.EnableEBT)
	}
	if UseConfigValue("nounixsock") {
		flagDisableUNIXSock = (bool)(config.NoUnixSocket)
	}
	if UseConfigValue("hmac") {
		hmacSec = config.Hmac
	}
	if UseConfigValue("debugdir") {
		debugLogDir = config.DebugDir
	}
	if UseConfigValue("debuglis") {
		debugAddr = config.MetricsAddress
	}
	if UseConfigValue("repair") {
		flagRepair = (bool)(config.RepairFSBeforeStart)
	}
}

func runSbot() error {
	initFlags()

	//log = logging.Logger("sbot")

	// 2022-02-22: cryptix wants to change away from NewRelativeTimeLogger because for long running code it doesn't make
	// sense; it's mostly a development convienence (which is why it's often used in the tests)
	log = testutils.NewRelativeTimeLogger(nil)

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

	// try to read config && environment variables, and apply any set values on variables that
	// have not been explicitly configured using flags on startup
	applyConfigValues()

	// add a log on is used by the sbot to aid ambient debugging for operators
	absRepo, err := filepath.Abs(repoDir)
	if err == nil {
		level.Info(log).Log("event", "set repo", "path", absRepo)
	}

	if debugLogDir != "" {
		logDir := filepath.Join(repoDir, debugLogDir)
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
		//logging.SetupLogging(os.Stderr)
	}

	ak, err := base64.StdEncoding.DecodeString(appKey)
	if err != nil {
		return fmt.Errorf("invalid application key/shs-cap: %w", err)
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
		mksbot.WithWebsocketAddress(wsLisAddr),
		// enabling this might consume a lot of resources
		mksbot.DisableLegacyLiveReplication(true),
		// new code, test with caution
		mksbot.DisableEBT(!flagEnableEBT),
	}

	if !flagDisableUNIXSock {
		opts = append(opts, mksbot.LateOption(mksbot.WithUNIXSocket()))
	}

	if debugLogDir != "" {
		opts = append(opts, mksbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
			parts := strings.Split(conn.RemoteAddr().String(), "|")

			if len(parts) != 2 {
				return conn, nil
			}

			muxrpcDumpDir := filepath.Join(
				repoDir,
				debugLogDir,
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
			return fmt.Errorf("invalid base64 string for HMAC signing secret: %w", err)
		}
		opts = append(opts, mksbot.WithHMACSigning(hcbytes))
	}

	sbot, err := mksbot.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to instantiate ssb server: %w", err)
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

	id := sbot.KeyPair.ID()
	uf, ok := sbot.GetMultiLog(multilogs.IndexNameFeeds)
	if !ok {
		checkAndLog(fmt.Errorf("missing userFeeds"))
		return nil
	}

	level.Info(log).Log("event", "waiting for indexes to catch up")
	sbot.WaitUntilIndexesAreSynced()

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
			return fmt.Errorf("fsck returned: %w", err)
		}

		switch report := err.(type) {
		case ssb.ErrWrongSequence:

			err = sbot.NullFeed(report.Ref)
			if err != nil {
				return fmt.Errorf("fsck: failed to drop broken feed: %w", err)
			}

			sbot.Shutdown()
			err := sbot.Close()
			if err != nil {
				return fmt.Errorf("fsck: failed to stop sbot after repair action: %w", err)
			}

		case mksbot.ErrConsistencyProblems:
			err = sbot.HealRepo(report)
			if err != nil {
				level.Error(log).Log("fsck", "heal failed", "err", err)
			} else {
				level.Info(log).Log("fsck", "healed",
					"msgs", report.Sequences.GetCardinality(),
					"feeds", len(report.Errors))
			}
			sbot.Shutdown()
			err := sbot.Close()
			if err != nil {
				return fmt.Errorf("fsck: failed to halt sbot after repo heal: %w", err)
			}
		default:
			level.Error(log).Log("fsck", "wrong report type", "T", fmt.Sprintf("%T", err))

		}

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
	// establish message anf feed numbers in the repo

	feeds, err := uf.List()
	if err != nil {
		return fmt.Errorf("user feed: %w", err)
	}
	RepoStats.With("part", "feeds").Set(float64(len(feeds)))

	msgCount := sbot.ReceiveLog.Seq() + 1
	RepoStats.With("part", "msgs").Set(float64(msgCount))

	level.Info(log).Log("event", "repo open", "feeds", len(feeds), "msgs", msgCount)

	if flagReindex {
		level.Warn(log).Log("mode", "reindexing")
		if fsckMode != mksbot.FSCKModeSequences {
			err = sbot.FSCK(mksbot.FSCKWithMode(mksbot.FSCKModeSequences))
			if err != nil {
				return err
			}
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
			return fmt.Errorf("failed to build graph during cleanup: %w", err)
		}

		botRef := sbot.KeyPair.ID()
		lst, err := tg.BlockedList(botRef).List()
		if err != nil {
			return fmt.Errorf("cleanup: failed to get blocked list: %w", err)
		}

		for _, blocked := range lst {
			isStored, err := multilog.Has(uf, storedrefs.Feed(blocked))
			if err != nil {
				return fmt.Errorf("blocked lookup in multilog: %w", err)
			}

			if isStored {
				level.Info(log).Log("event", "nulled feed", "ref", blocked.String())
				err = sbot.NullFeed(blocked)
				if err != nil {
					return fmt.Errorf("failed to null blocked feed %s: %w", blocked.String(), err)
				}
			}
		}

		sbot.Shutdown()
		return sbot.Close()
	}

	level.Info(log).Log("event", "serving", "ID", id.String(), "addr", listenAddr, "version", Version, "build", Build)
	for {
		// Note: This is where the serving starts ;)
		err = sbot.Network.Serve(ctx)
		if err != nil {
			level.Warn(log).Log("event", "sbot node.Serve returned", "err", err)
		}
		SystemEvents.With("event", "nodeServ exited").Add(1)
		time.Sleep(1 * time.Second)
		select {
		case <-ctx.Done():
			err := sbot.Close()
			return err
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
