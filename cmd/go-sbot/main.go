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
	"syscall"
	"time"

	"github.com/cryptix/go/logging"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/framing/lengthprefixed"
	"go.cryptoscope.co/margaret/offset"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
	"go.cryptoscope.co/sbot/graph"
	"go.cryptoscope.co/sbot/indexes"
	"go.cryptoscope.co/sbot/message"
	"go.cryptoscope.co/sbot/multilogs"
	"go.cryptoscope.co/sbot/plugins/blobs"
	"go.cryptoscope.co/sbot/plugins/gossip"
	"go.cryptoscope.co/sbot/plugins/whoami"
	"go.cryptoscope.co/sbot/repo"

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
	appKey   []byte
	localKey sbot.KeyPair
)

func checkAndLog(err error) {
	if err != nil {
		if err := logging.LogPanicWithStack(log, "checkAndLog", err); err != nil {
			panic(err)
		}
	}
}

func goThenLog(ctx context.Context, l margaret.Log, name string, f func(context.Context, margaret.Log) error) {
	go func() {
		err := f(ctx, l)
		if err == nil {
			log.Log("event", "component terminated without error", "component", name)
			return
		}

		log.Log("event", "component terminated", "component", name, "error", err)
	}()
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

	var (
		node    sbot.Node
		r       repo.Interface
		rootLog margaret.Log
		err     error

		closers multiCloser
	)

	r, err = repo.New(kitlog.With(log, "module", "repo"), repoDir)
	checkFatal(err)

	// no lock needed yet because the goroutine is not started yet
	closers.addCloser(r)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Log("event", "killed", "msg", "received signal, shutting down", "signal", sig.String())
		shutdown()

		err := closers.Close()
		checkAndLog(err)

		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	logging.SetCloseChan(c)

	rootLog, err = repo.GetRootLog(r)
	checkFatal(err)

	uf, _, serveUF, err := multilogs.GetUserFeeds(r)
	checkFatal(err)
	closers.addCloser(uf)
	goThenLog(ctx, rootLog, "userFeeds", serveUF)

	graphBuilder, serveContacts, err := indexes.GetContacts(kitlog.With(log, "index", "contacts"), r)
	checkFatal(err)
	closers.addCloser(graphBuilder)
	goThenLog(ctx, rootLog, "contacts", serveContacts)

	var (
		id = r.KeyPair().Id
		bs = r.BlobStore()
		wm = blobstore.NewWantManager(kitlog.With(log, "module", "WantManager"), bs)
	)

	feeds, err := uf.List()
	checkFatal(err)
	log.Log("event", "repo open", "feeds", len(feeds))
	for _, author := range feeds {
		subLog, err := uf.Get(author)
		checkFatal(err)

		currSeq, err := subLog.Seq().Value()
		checkFatal(err)

		authorRef := sbot.FeedRef{
			Algo: "ed25519",
			ID:   []byte(author),
		}
		f, err := graphBuilder.Follows(&authorRef)
		checkFatal(err)

		log.Log("info", "currSeq", "feed", authorRef.Ref(), "seq", currSeq, "follows", len(f))
	}

	localKey = r.KeyPair()

	pmgr := sbot.NewPluginManager()

	laddr, err := net.ResolveTCPAddr("tcp", listenAddr)
	checkFatal(err)

	// TODO get rid of this. either add error to pluginmgr.MakeHandler or take it away from the Options.
	errAdapter := func(mk func(net.Conn) muxrpc.Handler) func(net.Conn) (muxrpc.Handler, error) {
		return func(conn net.Conn) (muxrpc.Handler, error) {
			return mk(conn), nil
		}
	}

	opts := sbot.Options{
		ListenAddr:  laddr,
		KeyPair:     localKey,
		AppKey:      appKey,
		MakeHandler: graph.Authorize(kitlog.With(log, "module", "auth handler"), graphBuilder, localKey.Id, 2, errAdapter(pmgr.MakeHandler)),
	}

	node, err = sbot.NewNode(opts)
	checkFatal(err)

	pmgr.Register(whoami.New(kitlog.With(log, "plugin", "whoami"), localKey.Id)) // whoami
	pmgr.Register(blobs.New(kitlog.With(log, "plugin", "blobs"), bs, wm))        // blobs

	// gossip.*
	pmgr.Register(gossip.New(
		kitlog.With(log, "plugin", "gossip"),
		id, rootLog, uf, graphBuilder, node, flagPromisc))

	// createHistoryStream
	pmgr.Register(gossip.NewHist(
		kitlog.With(log, "plugin", "gossip/hist"),
		id, rootLog, uf, graphBuilder, node))

	log.Log("event", "serving", "ID", localKey.Id.Ref(), "addr", opts.ListenAddr)
	for {
		err = node.Serve(ctx)
		log.Log("event", "sbot node.Serve returned", "err", err)
		time.Sleep(1 * time.Second)
	}
}

func GetRootLog(r repo.Interface) (margaret.Log, error) {
	logFile, err := os.OpenFile(r.GetPath("log"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "error opening log file")
	}

	// TODO use proper log message type here
	// FIXME: 16kB because some messages are even larger than 12kB - even though the limit is supposed to be 8kb
	rootLog, err := offset.New(logFile, lengthprefixed.New32(16*1024), msgpack.New(&message.StoredMessage{}))
	return rootLog, errors.Wrap(err, "failed to create rootLog")
}
