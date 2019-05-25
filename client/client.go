package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/agl/ed25519"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/debug"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/message/legacy"
	"go.cryptoscope.co/ssb/plugins/replicate"
)

type client struct {
	muxrpc.Endpoint
	rootCtx       context.Context
	rootCtxCancel context.CancelFunc
	logger        log.Logger
}

// options?

const ssbAppkey = "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s="

func FromEndpoint(edp muxrpc.Endpoint) Interface {
	c := client{
		logger: log.With(log.NewLogfmtLogger(os.Stderr), "unit", "ssbClient"),
	}
	c.rootCtx, c.rootCtxCancel = context.WithCancel(context.TODO())
	c.Endpoint = edp
	return c
}

func NewTCP(ctx context.Context, own *ssb.KeyPair, remote net.Addr) (Interface, error) {
	c := client{
		logger: log.With(log.NewLogfmtLogger(os.Stderr), "unit", "ssbClient"),
	}
	c.rootCtx, c.rootCtxCancel = context.WithCancel(ctx)

	appKey, err := base64.StdEncoding.DecodeString(ssbAppkey)
	if err != nil {
		return nil, errors.Wrap(err, "ssbClient: failed to decode (default) appKey")
	}

	shsClient, err := secretstream.NewClient(own.Pair, appKey)
	if err != nil {
		return nil, errors.Wrap(err, "ssbClient: error creating secretstream.Client")
	}

	// todo: would be nice if netwrap could handle these two steps
	// but then it still needs to have the shsClient somehow
	shsAddr := netwrap.GetAddr(remote, "shs-bs")
	if shsAddr == nil {
		return nil, errors.New("ssbClient: expected an address containing an shs-bs addr")
	}

	var pubKey [ed25519.PublicKeySize]byte
	if shsAddr, ok := shsAddr.(secretstream.Addr); ok {
		copy(pubKey[:], shsAddr.PubKey)
	} else {
		return nil, errors.New("ssbClient: expected shs-bs address to be of type secretstream.Addr")
	}

	conn, err := netwrap.Dial(netwrap.GetAddr(remote, "tcp"), shsClient.ConnWrapper(pubKey))
	if err != nil {
		return nil, errors.Wrap(err, "error dialing")
	}

	h := noopHandler{
		logger: c.logger,
	}

	var rwc io.ReadWriteCloser = conn
	// logs every muxrpc packet
	if os.Getenv("MUXRPCDBUG") != "" {
		rwc = debug.Wrap(log.NewLogfmtLogger(os.Stderr), rwc)
	}

	c.Endpoint = muxrpc.HandleWithRemote(muxrpc.NewPacker(rwc), &h, conn.RemoteAddr())

	srv, ok := c.Endpoint.(muxrpc.Server)
	if !ok {
		return nil, errors.Errorf("ssbClient: failed to cast handler to muxrpc server (has type: %T)", c.Endpoint)
	}

	go func() {
		err := srv.Serve(c.rootCtx)
		c.logger.Log("warning", "muxrpc.Serve exited", "err", err)
	}()

	return &c, nil
}

func NewUnix(ctx context.Context, path string) (Interface, error) {
	c := client{
		logger: log.With(log.NewLogfmtLogger(os.Stderr), "unit", "ssbClient"),
	}
	c.rootCtx, c.rootCtxCancel = context.WithCancel(ctx)

	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, errors.Errorf("ssbClient: failed to open unix path")
	}
	h := noopHandler{
		logger: c.logger,
	}

	var rwc io.ReadWriteCloser = conn
	// logs every muxrpc packet
	if os.Getenv("MUXRPCDBUG") != "" {
		rwc = debug.Wrap(log.NewLogfmtLogger(os.Stderr), rwc)
	}

	c.Endpoint = muxrpc.Handle(muxrpc.NewPacker(rwc), &h)

	srv, ok := c.Endpoint.(muxrpc.Server)
	if !ok {
		return nil, errors.Errorf("ssbClient: failed to cast handler to muxrpc server (has type: %T)", c.Endpoint)
	}

	go func() {
		err := srv.Serve(c.rootCtx)
		c.logger.Log("warning", "muxrpc.Serve exited", "err", err)
	}()

	return &c, nil
}

func (c client) Close() error {
	c.rootCtxCancel()
	return nil
}

func (c client) Whoami() (*ssb.FeedRef, error) {
	v, err := c.Async(c.rootCtx, message.WhoamiReply{}, muxrpc.Method{"whoami"})
	if err != nil {
		return nil, errors.Wrap(err, "ssbClient: whoami failed")
	}
	resp, ok := v.(message.WhoamiReply)
	if !ok {
		return nil, errors.Errorf("ssbClient: wrong response type: %T", v)
	}
	return resp.ID, nil
}

func (c client) BlobsWant(ref ssb.BlobRef) error {
	var v interface{}
	v, err := c.Async(c.rootCtx, v, muxrpc.Method{"blobs", "want"}, ref.Ref())
	if err != nil {
		return errors.Wrap(err, "ssbClient: whoami failed")
	}
	c.logger.Log("blob", "wanted", "v", v, "ref", ref.Ref())
	return nil
}

func (c client) Publish(v interface{}) (*ssb.MessageRef, error) {
	v, err := c.Async(c.rootCtx, "str", muxrpc.Method{"publish"}, v)
	if err != nil {
		return nil, errors.Wrap(err, "ssbClient: whoami failed")
	}
	resp, ok := v.(string)
	if !ok {
		return nil, errors.Errorf("ssbClient: wrong reply type: %T", v)
	}
	msgRef, err := ssb.ParseMessageRef(resp)
	return msgRef, errors.Wrap(err, "failed to parse new message reference")
}

func (c client) CreateLogStream(opts message.CreateHistArgs) (luigi.Source, error) {
	opts.Keys = true
	src, err := c.Source(c.rootCtx, legacy.KeyValueAsMap{}, muxrpc.Method{"createLogStream"}, opts)
	return src, errors.Wrap(err, "failed to create stream")
}

func (c client) CreateHistoryStream(o message.CreateHistArgs, as interface{}) (luigi.Source, error) {
	src, err := c.Source(c.rootCtx, as, muxrpc.Method{"createHistoryStream"}, o)
	return src, errors.Wrap(err, "failed to create stream")
}

func (c client) ReplicateUpTo() (luigi.Source, error) {
	src, err := c.Source(c.rootCtx, replicate.UpToResponse{}, muxrpc.Method{"replicate", "upto"})
	return src, errors.Wrap(err, "failed to create stream")
}

func (c client) Tangles(root ssb.MessageRef, o message.CreateHistArgs) (luigi.Source, error) {
	var opt struct {
		message.CreateHistArgs
		Root string `json:"root"`
	}
	opt.CreateHistArgs = o
	opt.Keys = true
	opt.Root = root.Ref()
	src, err := c.Source(c.rootCtx, legacy.KeyValueAsMap{}, muxrpc.Method{"tangles"}, opt)
	return src, errors.Wrap(err, "failed to create stream")
}

type noopHandler struct {
	logger log.Logger
}

func (h noopHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	srv := edp.(muxrpc.Server)
	h.logger.Log("event", "onConnect", "addr", srv.Remote())
}

func (h noopHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	h.logger.Log("event", "onCall", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
}
