package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"os/user"
	"path/filepath"

	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/sbot"
	"cryptoscope.co/go/sbot/repo"
	"cryptoscope.co/go/secretstream/secrethandshake"
	"github.com/cryptix/go/logging"
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
	localKey secrethandshake.EdKeyPair
	localID  *sbot.FeedRef
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

	localKey = r.KeyPair()
	localID = &sbot.FeedRef{ID: localKey.Public[:], Algo: "ed25519"}

	rootHdlr := &muxrpc.HandlerMux{}
	rootHdlr.Register(muxrpc.Method{"whoami"}, whoAmI{I: *localID})
	rootHdlr.Register(muxrpc.Method{"gossip"}, &connect{&node})

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

	log.Log("event", "serving", "ID", localID.Ref(), "addr", opts.ListenAddr)
	err = node.Serve(ctx)
	checkFatal(err)
}
