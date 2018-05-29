package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"strings"

	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/netwrap"
	"cryptoscope.co/go/sbot"
	"cryptoscope.co/go/secretstream"
	"cryptoscope.co/go/secretstream/secrethandshake"
	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
)

type whoAmIEndpoint struct {
	muxrpc.Endpoint
}

var (
	listenAddr  string
	connectAddr string
	appKey      []byte
	seed        int64

	log        logging.Interface
	checkFatal = logging.CheckFatal
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

	flag.StringVar(&listenAddr, "l", ":8008", "address to listen on")
	flag.Int64Var(&seed, "seed", 42, "number to seed key generation (reproducible and insecure!)")
	flag.StringVar(&connectAddr, "connect", "", "address to connect to after startup")

	flag.Parse()
}

func main() {
	ctx := context.Background()

	var (
		node sbot.Node
		err  error
	)

	keyPair, err := secrethandshake.GenEdKeyPair(rand.New(rand.NewSource(seed)))
	checkFatal(err)

	rootHdlr := &muxrpc.HandlerMux{}
	rootHdlr.Register(muxrpc.Method{"whoami"}, whoAmI{PubKey: keyPair.Public[:]})
	rootHdlr.Register(muxrpc.Method{"connect"}, &connect{&node})

	laddr, err := net.ResolveTCPAddr("tcp", listenAddr)
	checkFatal(err)

	opts := sbot.Options{
		ListenAddr:  laddr,
		KeyPair:     *keyPair,
		AppKey:      appKey,
		MakeHandler: func(net.Conn) muxrpc.Handler { return rootHdlr },
	}

	node, err = sbot.NewNode(opts)
	checkFatal(err)

	// initial peer specified on cli. this will go once we actually do stuff
	if connectAddr != "" {
		split := strings.Split(connectAddr, "/")

		tcpAddrStr := split[0]
		tcpAddr, err := net.ResolveTCPAddr("tcp", tcpAddrStr)
		checkFatal(errors.Wrapf(err, "error resolving network address %s", tcpAddrStr))

		hexPubKey := split[1]
		pubKey, err := hex.DecodeString(hexPubKey)
		checkFatal(errors.Wrapf(err, "error decoding hex string %q", hexPubKey))

		ssAddr := secretstream.Addr{PubKey: pubKey}
		err = node.Connect(ctx, netwrap.WrapAddr(tcpAddr, ssAddr))
		checkFatal(errors.Wrapf(err, "error connecting to %q", connectAddr))
	}

	log.Log("event", "serving", "key", fmt.Sprintf("%x", keyPair.Public), "addr", opts.ListenAddr)
	err = node.Serve(ctx)
	checkFatal(err)
}
