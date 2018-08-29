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
	"syscall"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
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
		node sbot.Node
		r    repo.Interface
		err  error
	)

	r, err = repo.New(repoDir)
	checkFatal(err)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Log("event", "killed", "msg", "received signal, shutting down", "signal", sig.String())
		shutdown()
		checkFatal(r.Close())
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	logging.SetCloseChan(c)

	uf := r.UserFeeds()
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
		f, err := r.IsFollowing(&authorRef)
		checkFatal(err)

		log.Log("info", "currSeq", "feed", authorRef.Ref(), "seq", currSeq, "follows", len(f))

	}

	localKey = r.KeyPair()

	pmgr := sbot.NewPluginManager()

	laddr, err := net.ResolveTCPAddr("tcp", listenAddr)
	checkFatal(err)

	opts := sbot.Options{
		ListenAddr: laddr,
		KeyPair:    localKey,
		AppKey:     appKey,
		MakeHandler: func(conn net.Conn) (muxrpc.Handler, error) {
			if len(feeds) != 0 { // trust on first use
				remote, err := sbot.GetFeedRefFromAddr(conn.RemoteAddr())
				if err != nil {
					return nil, errors.Wrap(err, "MakeHandler: expected an address containing an shs-bs addr")
				}

				// TODO: cache me in tandem with indexing
				timeGraph := time.Now()

				fg, err := r.Makegraph()
				if err != nil {
					return nil, errors.Wrap(err, "MakeHandler: failed to make friendgraph")
				}
				timeDijkstra := time.Now()

				if fg.IsFollowing(&localKey.Id, remote) {
					// quick skip direct follow
					return pmgr.MakeHandler(conn), nil
				}

				distLookup, err := fg.MakeDijkstra(&localKey.Id)
				if err != nil {
					return nil, errors.Wrap(err, "MakeHandler: failed to construct dijkstra")
				}
				timeLookup := time.Now()

				fpath, d := distLookup.Dist(remote)
				timeDone := time.Now()

				log.Log("event", "disjkstra",
					"nodes", fg.Nodes(),

					"total", timeDone.Sub(timeGraph),
					"lookup", timeDone.Sub(timeLookup),
					"mkGraph", timeDijkstra.Sub(timeGraph),
					"mkSearch", timeLookup.Sub(timeDijkstra),

					"dist", d,
					"hops", len(fpath),
					"path", fmt.Sprint(fpath),

					"remote", remote,
				)

				if d < 0 && len(fpath) < 3 {
					return nil, errors.Errorf("sbot: peer not in reach. d:%f", d)
				}
			}

			return pmgr.MakeHandler(conn), nil
		},
	}

	node, err = sbot.NewNode(opts)
	checkFatal(err)

	bs := r.BlobStore()
	wm := blobstore.NewWantManager(log, bs)

	pmgr.Register(whoami.New(r))                         // whoami
	pmgr.Register(blobs.New(bs, wm))                     // blobs
	pmgr.Register(gossip.New(r, node, flagPromisc, log)) // gossip.*
	pmgr.Register(gossip.NewHist(r, node, log))          // createHistoryStream

	log.Log("event", "serving", "ID", localKey.Id.Ref(), "addr", opts.ListenAddr)
	for {
		err = node.Serve(ctx)
		log.Log("event", "sbot node.Serve returned", "err", err)
		time.Sleep(1 * time.Second)
	}
}
