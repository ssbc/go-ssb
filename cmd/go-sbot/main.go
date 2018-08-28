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
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
	"go.cryptoscope.co/sbot/plugins/blobs"
	"go.cryptoscope.co/sbot/plugins/gossip"
	"go.cryptoscope.co/sbot/plugins/whoami"
	"go.cryptoscope.co/sbot/repo"
	"go.cryptoscope.co/secretstream"

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
		r    sbot.Repo
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
		has, err := multilog.Has(uf, author)
		checkFatal(err)

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

	var peerWhitelist = map[string]bool{ // TODO: add yours here - see below
		"@38N97KFM3f9MBFBVE/9HwQsECm6G/AmGqViG8joZQ44=.ed25519": true,
		"@uOReuhnb9+mPi5RnTbKMKRr3r87cK+aOg8lFXV/SBPU=.ed25519": true,
		"@Q7a4cAeHiezNbBFAuZZxDG3jugCgOfhpUHqLJBSD2mQ=.ed25519": true,
		"@So2yhYGA2ZOwRh2043whISASD+55PL1P9+peIy/qbj8=.ed25519": true, // heropunch
		"@DTNmX+4SjsgZ7xyDh5xxmNtFqa6pWi5Qtw7cE8aR9TQ=.ed25519": true, // wx.larpa.net
		"@WndnBREUvtFVF14XYEq01icpt91753bA+nVycEJIAX4=.ed25519": true, // t4l3.net
	}
	peerWhitelist[localKey.Id.Ref()] = true // allow self

	opts := sbot.Options{
		ListenAddr: laddr,
		KeyPair:    localKey,
		AppKey:     appKey,
		MakeHandler: func(conn net.Conn) (muxrpc.Handler, error) {
			addr := netwrap.GetAddr(conn.RemoteAddr(), "shs-bs")
			if addr == nil {
				return nil, errors.New("expected an address containing an shs-bs addr")
			}

			secstreamAddr, ok := addr.(secretstream.Addr)
			if !ok {
				return nil, errors.Errorf("unexpected shs-bs addr type: %T", addr)
			}
			fr := sbot.FeedRef{
				Algo: "ed25519",
				ID:   secstreamAddr.PubKey,
			}

			if ok := peerWhitelist[fr.Ref()]; !ok {
				return nil, errors.New("sbot: not whitelisted - peer locked down for testing")
			}

			// TODO: check remote key is in friend-graph distance
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
