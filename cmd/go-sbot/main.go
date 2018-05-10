package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"

	"cryptoscope.co/go/muxrpc"
	"cryptoscope.co/go/netwrap"
	"cryptoscope.co/go/sbot"
	"cryptoscope.co/go/secretstream"
	"cryptoscope.co/go/secretstream/secrethandshake"
)

type whoAmIEndpoint struct {
	muxrpc.Endpoint
}

var (
	listenPort  int
	connectAddr string
	appKey      []byte
	seed        int64
)

func init() {
	var err error
	appKey, err = base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
	if err != nil {
		panic(err)
	}

	flag.IntVar(&listenPort, "l", 8008, "port to listen on")
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
	if err != nil {
		panic(err)
	}

	rootHdlr := &muxrpc.HandlerMux{}
	rootHdlr.Register(muxrpc.Method{"whoami"}, whoAmI{PubKey: keyPair.Public[:]})
	rootHdlr.Register(muxrpc.Method{"connect"}, &connect{&node})

	opts := sbot.Options{
		ListenAddr:  &net.TCPAddr{Port: listenPort},
		KeyPair:     *keyPair,
		AppKey:      appKey,
		MakeHandler: func(net.Conn) muxrpc.Handler { return rootHdlr },
	}

	node, err = sbot.NewNode(opts)
	if err != nil {
		panic(err)
	}

	// initial peer specified on cli. this will go once we actually do stuff
	if connectAddr != "" {
		split := strings.Split(connectAddr, "/")

		tcpAddrStr := split[0]
		tcpAddr, err := net.ResolveTCPAddr("tcp", tcpAddrStr)
		if err != nil {
			log.Printf("error resolving network address %q: %s\n", tcpAddrStr, err)
			return
		}

		hexPubKey := split[1]
		pubKey, err := hex.DecodeString(hexPubKey)
		if err != nil {
			log.Printf("error decoding hex string %q: %s", hexPubKey, err)
		}
		ssAddr := secretstream.Addr{PubKey: pubKey}

		err = node.Connect(ctx, netwrap.WrapAddr(tcpAddr, ssAddr))
		if err != nil {
			log.Printf("error connecting to %q: %s\n", connectAddr, err)
			return
		}
	}

	fmt.Printf("serving key %x on %s\n", keyPair.Public, opts.ListenAddr)
	err = node.Serve(ctx)
	if err != nil {
		panic(err)
	}
}
