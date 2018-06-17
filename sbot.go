package sbot

import (
	"context"
	"log"
	"net"

	"github.com/agl/ed25519"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
)

type Options struct {
	ListenAddr  net.Addr
	KeyPair     KeyPair
	AppKey      []byte
	MakeHandler func(net.Conn) muxrpc.Handler
}

type Node interface {
	Connect(ctx context.Context, addr net.Addr) error
	Serve(ctx context.Context) error
}

type node struct {
	opts Options

	l            net.Listener
	secretServer *secretstream.Server
	secretClient *secretstream.Client
	connTracker  ConnTracker
}

func NewNode(opts Options) (Node, error) {
	n := &node{
		opts:        opts,
		connTracker: NewConnTracker(),
	}

	var err error

	n.secretClient, err = secretstream.NewClient(opts.KeyPair.Pair, opts.AppKey)
	if err != nil {
		return nil, errors.Wrap(err, "error creating secretstream.Client")
	}

	n.secretServer, err = secretstream.NewServer(opts.KeyPair.Pair, opts.AppKey)
	if err != nil {
		return nil, errors.Wrap(err, "error creating secretstream.Server")
	}

	n.l, err = netwrap.Listen(n.opts.ListenAddr, n.secretServer.ListenerWrapper())
	if err != nil {
		return nil, errors.Wrap(err, "error creating listener")
	}
	return n, nil
}

func (n *node) handleConnection(ctx context.Context, conn net.Conn) {
	h := n.opts.MakeHandler(conn)
	pkr := muxrpc.NewPacker(conn) //codec.Wrap(logging.Logger("handleC"), conn))

	n.connTracker.OnAccept(conn)
	defer n.connTracker.OnClose(conn)

	edp := muxrpc.HandleWithRemote(pkr, h, conn.RemoteAddr())
	go h.HandleConnect(ctx, edp)

	srv := edp.(muxrpc.Server)
	err := srv.Serve(ctx)
	if err != nil {
		log.Printf("error serving muxrpc session with peer %s: %s", conn.RemoteAddr(), err)
	}
}

func (n *node) Serve(ctx context.Context) error {
	for {
		conn, err := n.l.Accept()
		if err != nil {
			return errors.Wrap(err, "error accepting connection")
		}
		go func(c net.Conn) {
			n.handleConnection(ctx, c)
		}(conn)
	}
}

func (n *node) Connect(ctx context.Context, addr net.Addr) error {
	shsAddr := netwrap.GetAddr(addr, "shs-bs")
	if shsAddr == nil {
		return errors.New("expected an address containing an shs-bs addr")
	}

	var pubKey [ed25519.PublicKeySize]byte
	if shsAddr, ok := shsAddr.(secretstream.Addr); ok {
		copy(pubKey[:], shsAddr.PubKey)
	} else {
		return errors.New("expected shs-bs address to be of type secretstream.Addr")
	}

	conn, err := netwrap.Dial(netwrap.GetAddr(addr, "tcp"), n.secretClient.ConnWrapper(pubKey))
	if err != nil {
		return errors.Wrap(err, "error dialing")
	}

	go func(c net.Conn) {
		n.handleConnection(ctx, c)
	}(conn)
	return nil
}
