package sbot

import (
	"context"
	"net"

	"github.com/agl/ed25519"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
)

type Options struct {
	ListenAddr   net.Addr
	KeyPair      *KeyPair
	AppKey       []byte
	MakeHandler  func(net.Conn) (muxrpc.Handler, error)
	Logger       log.Logger
	ConnWrappers []netwrap.ConnWrapper
}

type Node interface {
	Connect(ctx context.Context, addr net.Addr) error
	Serve(ctx context.Context) error
	GetListenAddr() net.Addr
}

type node struct {
	opts Options

	l            net.Listener
	secretServer *secretstream.Server
	secretClient *secretstream.Client
	connTracker  ConnTracker
	log          log.Logger
	connWrappers []netwrap.ConnWrapper
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

	n.connWrappers = opts.ConnWrappers
	n.log = opts.Logger

	return n, nil
}

func (n *node) handleConnection(ctx context.Context, conn net.Conn) {
	n.connTracker.OnAccept(conn)
	defer n.connTracker.OnClose(conn)

	h, err := n.opts.MakeHandler(conn)
	if err != nil {
		n.log.Log("func", "handleConnection", "op", "MakeHandler", "error", err.Error(), "peer", conn.RemoteAddr())
		return
	}

	pkr := muxrpc.NewPacker(conn)
	edp := muxrpc.HandleWithRemote(pkr, h, conn.RemoteAddr())

	srv := edp.(muxrpc.Server)
	if err := srv.Serve(ctx); err != nil {
		n.log.Log("func", "handleConnection", "op", "Serve", "error", err.Error(), "peer", conn.RemoteAddr())
	}
}

func (n *node) Serve(ctx context.Context) error {
	for {
		conn, err := n.l.Accept()
		if err != nil {
			return errors.Wrap(err, "error accepting connection")
		}

		n.log.Log("action", "applying connection wrappers", "count", len(n.connWrappers))
		// apply connection wrappers
		for i, cw := range n.connWrappers {
			var err error
			conn, err = cw(conn)
			if err != nil {
				return errors.Wrapf(err, "error applying connection wrapper #%d", i)
			}
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

	n.log.Log("action", "applying connection wrappers", "count", len(n.connWrappers))
	// apply connection wrappers
	for i, cw := range n.connWrappers {
		var err error
		conn, err = cw(conn)
		if err != nil {
			return errors.Wrapf(err, "error applying connection wrapper #%d", i)
		}
	}

	go func(c net.Conn) {
		n.handleConnection(ctx, c)
	}(conn)
	return nil
}

func (n *node) GetListenAddr() net.Addr {
	return n.l.Addr()
}
