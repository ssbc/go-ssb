package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"os/user"
	"path/filepath"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/gossip"
	"go.cryptoscope.co/sbot/repo"
)

var (
	// flags
	listenAddr string
	repoDir    string

	// helper
	log        logging.Interface
	checkFatal = logging.CheckFatal

	// juicy bits
	appKey   []byte
	localKey sbot.KeyPair
)

func checkAndLog(err error) {
	if err != nil {
		log.Log("event", "error", "err", err)
		// TODO: push panic writer to go/logging
		fmt.Printf("Stack: %+v\n", err)
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
	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb"), "where to put the log and indexes")

	flag.Parse()
}

func main() {
	ctx := context.Background()

	var (
		node sbot.Node
		r    sbot.Repo
		err  error
	)

	r, err = repo.New(repoDir)
	checkFatal(err)

	m, err := r.KnownFeeds()
	checkFatal(err)
	//goon.Dump(m)
	for author, seq := range m {
		ref, err := sbot.ParseRef(author)
		checkFatal(err)
		fr := ref.(*sbot.FeedRef)

		/*
			seqs, err := r.FeedSeqs(*fr)
			checkFatal(err)
		*/

		fmt.Printf("known FeedRef:%s %d\n", fr.Ref(), seq)
	}

	localKey = r.KeyPair()

	rootHdlr := &muxrpc.HandlerMux{}

	laddr, err := net.ResolveTCPAddr("tcp", listenAddr)
	checkFatal(err)

	opts := sbot.Options{
		ListenAddr:  laddr,
		KeyPair:     localKey,
		AppKey:      appKey,
		MakeHandler: func(net.Conn) muxrpc.Handler { return rootHdlr },
	}

	node, err = sbot.NewNode(opts)
	checkFatal(err)

	gossipHandler := &gossip.Handler{
		Node: node,
		Repo: r,
		Info: log,
	}
	rootHdlr.Register(muxrpc.Method{"whoami"}, whoAmI{I: localKey.Id})
	rootHdlr.Register(muxrpc.Method{"gossip"}, gossipHandler)
	rootHdlr.Register(muxrpc.Method{"createHistoryStream"}, gossipHandler)

	log.Log("event", "serving", "ID", localKey.Id.Ref(), "addr", opts.ListenAddr)
	err = node.Serve(ctx)
	checkFatal(err)
}
