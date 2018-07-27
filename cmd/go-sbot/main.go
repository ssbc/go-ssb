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
	"go.cryptoscope.co/sbot/plugins/gossip"
	"go.cryptoscope.co/sbot/repo"
	"go.cryptoscope.co/secretstream"
)

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
	logging.SetupLogging(nil)
	log = logging.Logger("sbot")

	var err error
	appKey, err = base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
	checkFatal(err)

	u, err := user.Current()
	checkFatal(err)

	flag.StringVar(&listenAddr, "l", ":8008", "address to listen on")
	flag.BoolVar(&flagPromisc, "promisc", false, "crawl all the feeds")
	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb"), "where to put the log and indexes")

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
		log.Log("info", "currSeq", "feed", authorRef.Ref(), "seq", currSeq, "has", has)
	}

	localKey = r.KeyPair()

	rootHdlr := &muxrpc.HandlerMux{}

	laddr, err := net.ResolveTCPAddr("tcp", listenAddr)
	checkFatal(err)

	var peerWhitelist = map[string]bool{ // TODO: add yours here - see below
		"@38N97KFM3f9MBFBVE/9HwQsECm6G/AmGqViG8joZQ44=.ed25519": true,
		"@uOReuhnb9+mPi5RnTbKMKRr3r87cK+aOg8lFXV/SBPU=.ed25519": true,
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
			return rootHdlr, nil
		},
	}

	node, err = sbot.NewNode(opts)
	checkFatal(err)

	gossipHandler := &gossip.Handler{
		Node:    node,
		Repo:    r,
		Info:    log,
		Promisc: flagPromisc,
	}
	rootHdlr.Register(muxrpc.Method{"whoami"}, whoAmI{I: localKey.Id})
	rootHdlr.Register(muxrpc.Method{"gossip"}, gossipHandler)
	rootHdlr.Register(muxrpc.Method{"createHistoryStream"}, IgnoreConnectHandler{gossipHandler})

	log.Log("event", "serving", "ID", localKey.Id.Ref(), "addr", opts.ListenAddr)
	for {
		err = node.Serve(ctx)
		log.Log("event", "sbot node.Serve returned", "err", err)
		time.Sleep(1 * time.Second)
	}
}

type IgnoreConnectHandler struct{ muxrpc.Handler }

func (IgnoreConnectHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}
