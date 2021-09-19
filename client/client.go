// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// Package client is a a simple muxrpc interface to common ssb methods, similar to npm:ssb-client
package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"
	"golang.org/x/crypto/ed25519"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/plugins/whoami"
	refs "go.mindeco.de/ssb-refs"
)

// Client exposes the underlying muxrpc endpoint(client) together with some high-level utility functions for common tasks
type Client struct {
	muxrpc.Endpoint
	rootCtx       context.Context
	rootCtxCancel context.CancelFunc
	logger        log.Logger

	closer io.Closer

	appKeyBytes []byte
}

func newClientWithOptions(opts []Option) (*Client, error) {
	var c Client
	for i, o := range opts {
		err := o(&c)
		if err != nil {
			return nil, fmt.Errorf("client: option #%d failed: %w", i, err)
		}
	}

	// defaults
	if c.logger == nil {
		defLog := log.With(log.NewLogfmtLogger(os.Stderr), "unit", "ssbClient")
		c.logger = level.NewFilter(defLog, level.AllowInfo())
	}

	if c.rootCtx == nil {
		c.rootCtx = context.Background()
	}
	c.rootCtx, c.rootCtxCancel = context.WithCancel(c.rootCtx)

	if c.appKeyBytes == nil {
		var err error
		c.appKeyBytes, err = base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
		if err != nil {
			return nil, fmt.Errorf("client: failed to decode default app key: %w", err)
		}
	}

	return &c, nil
}

/* TODO: add some tests to check isServer is working as expected
func FromEndpoint(edp muxrpc.Endpoint, opts ...Option) (*Client, error) {
	c, err := newClientWithOptions(opts)
	if err != nil {
		return nil, err
	}
	panic("TODO: is server?")
	c.Endpoint = edp
	return c, nil
}
*/

// NewTCP dials the remote via TCP/IP and returns a Client if the handshake succeeds.
// The remote's public-key needs to be added to the remote via the netwrap package and secretstream.Addr.
func NewTCP(own ssb.KeyPair, remote net.Addr, opts ...Option) (*Client, error) {
	c, err := newClientWithOptions(opts)
	if err != nil {
		return nil, err
	}

	edkp := ssb.EdKeyPair(own)

	shsClient, err := secretstream.NewClient(edkp, c.appKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("ssbClient: error creating secretstream.Client: %w", err)
	}

	// todo: would be nice if netwrap could handle these two steps
	// but then it still needs to have the shsClient somehow
	boxAddr := netwrap.GetAddr(remote, "shs-bs")
	if boxAddr == nil {
		return nil, errors.New("ssbClient: expected an address containing an shs-bs addr")
	}

	var pubKey = make(ed25519.PublicKey, ed25519.PublicKeySize)
	shsAddr, ok := boxAddr.(secretstream.Addr)
	if !ok {
		return nil, errors.New("ssbClient: expected shs-bs address to be of type secretstream.Addr")
	}
	copy(pubKey[:], shsAddr.PubKey)

	conn, err := netwrap.Dial(netwrap.GetAddr(remote, "tcp"), shsClient.ConnWrapper(pubKey))
	if err != nil {
		return nil, fmt.Errorf("error dialing: %w", err)
	}
	c.closer = conn

	h := whoami.New(c.logger, own.ID()).Handler()

	c.Endpoint = muxrpc.Handle(muxrpc.NewPacker(conn), h,
		muxrpc.WithIsServer(true),
		muxrpc.WithContext(c.rootCtx),
		muxrpc.WithRemoteAddr(conn.RemoteAddr()),
	)

	srv, ok := c.Endpoint.(muxrpc.Server)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("ssbClient: failed to cast handler to muxrpc server (has type: %T)", c.Endpoint)
	}

	go func() {
		err := srv.Serve()
		if err != nil {
			level.Warn(c.logger).Log("event", "muxrpc.Serve exited", "err", err)
		}
		conn.Close()
	}()

	return c, nil
}

// NewUnix creates a Client using a local unix socket file.
func NewUnix(path string, opts ...Option) (*Client, error) {
	c, err := newClientWithOptions(opts)
	if err != nil {
		return nil, err
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("ssbClient: failed to open unix path %q", path)
	}
	c.closer = conn

	h := noopHandler{
		logger: c.logger,
	}

	c.Endpoint = muxrpc.Handle(muxrpc.NewPacker(conn), &h,
		muxrpc.WithIsServer(true),
		muxrpc.WithContext(c.rootCtx),
	)

	srv, ok := c.Endpoint.(muxrpc.Server)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("ssbClient: failed to cast handler to muxrpc server (has type: %T)", c.Endpoint)
	}

	go func() {
		err := srv.Serve()
		if err != nil {
			level.Warn(c.logger).Log("event", "muxrpc.Serve exited", "err", err)
		}
		conn.Close()
	}()

	return c, nil
}

// Close closes the client and terminates all running requests.
func (c Client) Close() error {
	c.rootCtxCancel()
	c.Endpoint.Terminate()
	c.closer.Close()
	return nil
}

// Whoami returns the feed of the bot
func (c Client) Whoami() (refs.FeedRef, error) {
	var resp message.WhoamiReply
	err := c.Async(c.rootCtx, &resp, muxrpc.TypeJSON, muxrpc.Method{"whoami"})
	if err != nil {
		return refs.FeedRef{}, fmt.Errorf("ssbClient: whoami failed: %w", err)
	}
	return resp.ID, nil
}

// ReplicateUpTo returns a source where each element is a feed with it's length
func (c Client) ReplicateUpTo() (*muxrpc.ByteSource, error) {
	src, err := c.Source(c.rootCtx, muxrpc.TypeJSON, muxrpc.Method{"replicate", "upto"})
	if err != nil {
		return nil, fmt.Errorf("ssbClient: failed to create stream: %w", err)
	}
	return src, nil
}

// BlobsWant tells the remote to add a blob to its wantlist
func (c Client) BlobsWant(ref refs.BlobRef) error {
	var v interface{}
	err := c.Async(c.rootCtx, &v, muxrpc.TypeJSON, muxrpc.Method{"blobs", "want"}, ref.Sigil())
	if err != nil {
		return fmt.Errorf("ssbClient: blobs.want failed: %w", err)
	}
	level.Debug(c.logger).Log("blob", "wanted", "v", v, "ref", ref.Sigil())
	return nil
}

// BlobsHas checks if a blob is stored or not
func (c Client) BlobsHas(ref refs.BlobRef) (bool, error) {
	var has bool
	err := c.Async(c.rootCtx, &has, muxrpc.TypeJSON, muxrpc.Method{"blobs", "want"}, ref.Sigil())
	if err != nil {
		return false, fmt.Errorf("ssbClient: whoami failed: %w", err)
	}
	level.Debug(c.logger).Log("blob", "has", "has", has, "ref", ref.Sigil())
	return has, nil

}

// BlobsGet returns a reader for the data referenced by the blob
func (c Client) BlobsGet(ref refs.BlobRef) (io.Reader, error) {
	args := blobstore.GetWithSize{Key: ref, Max: blobstore.DefaultMaxSize}
	v, err := c.Source(c.rootCtx, 0, muxrpc.Method{"blobs", "get"}, args)
	if err != nil {
		return nil, fmt.Errorf("ssbClient: blobs.get failed: %w", err)
	}
	level.Debug(c.logger).Log("blob", "got", "ref", ref.Sigil())

	return muxrpc.NewSourceReader(v), nil
}

// NamesGetResult holds all the naming information available.
// It's a too stage map because it also contains names for feeds by other people.
type NamesGetResult map[string]map[string]string

// GetCommonName filters the map for name for that feed.
// Currently, if there is no self given name for a feed,
// this doesn't check if the the prescribed name is given by a friend or fow (TODO).
func (ngr NamesGetResult) GetCommonName(feed refs.FeedRef) (string, bool) {
	namesFor, ok := ngr[feed.Sigil()]
	if !ok {
		return "", false
	}

	selfChosen, ok := namesFor[feed.Sigil()]
	if ok {
		return selfChosen, true
	}

	// pick the first at random
	for _, prescribed := range namesFor {
		return prescribed, true
		// TODO: check that from is a friend or not blocked
	}

	return "", false
}

// NamesGet returns all the names for feeds
func (c Client) NamesGet() (NamesGetResult, error) {
	var res NamesGetResult
	err := c.Async(c.rootCtx, &res, muxrpc.TypeJSON, muxrpc.Method{"names", "get"})
	if err != nil {
		return nil, fmt.Errorf("ssbClient: names.get failed: %w", err)
	}
	level.Debug(c.logger).Log("names", "get", "cnt", len(res))
	return res, nil
}

// NamesSignifier mirrors ssb-names, returns the name for a feed
func (c Client) NamesSignifier(ref refs.FeedRef) (string, error) {
	var name string
	err := c.Async(c.rootCtx, &name, muxrpc.TypeString, muxrpc.Method{"names", "getSignifier"}, ref.String())
	if err != nil {
		return "", fmt.Errorf("ssbClient: names.getSignifier failed: %w", err)
	}
	level.Debug(c.logger).Log("names", "getSignifier", "name", name, "ref", ref.String())
	return name, nil
}

// NamesImageFor mirrors ssb-names, returns the avatar image for a feed
func (c Client) NamesImageFor(ref refs.FeedRef) (refs.BlobRef, error) {
	var blobRef string
	err := c.Async(c.rootCtx, &blobRef, muxrpc.TypeString, muxrpc.Method{"names", "getImageFor"}, ref.String())
	if err != nil {
		return refs.BlobRef{}, fmt.Errorf("ssbClient: names.getImageFor failed: %w", err)
	}
	level.Debug(c.logger).Log("names", "getImageFor", "image-blob", blobRef, "feed", ref.String())
	return refs.ParseBlobRef(blobRef)
}

// Publish publishes a new message, the passed value is the content.
func (c Client) Publish(v interface{}) (refs.MessageRef, error) {
	var resp string
	err := c.Async(c.rootCtx, &resp, muxrpc.TypeString, muxrpc.Method{"publish"}, v)
	if err != nil {
		return refs.MessageRef{}, fmt.Errorf("ssbClient: publish call failed: %w", err)
	}
	msgRef, err := refs.ParseMessageRef(resp)
	if err != nil {
		return refs.MessageRef{}, fmt.Errorf("failed to parse new message reference: %w", err)
	}
	return msgRef, nil
}

// PrivatePublish publishes an encrypted message to the recipients.
func (c Client) PrivatePublish(v interface{}, recps ...refs.FeedRef) (refs.MessageRef, error) {
	if len(recps) == 0 {
		return refs.MessageRef{}, fmt.Errorf("ssbClient: no recipients for new private message")
	}
	var recpRefs = make([]string, len(recps))
	for i, ref := range recps {
		recpRefs[i] = ref.String()
	}
	var resp string
	err := c.Async(c.rootCtx, &resp, muxrpc.TypeString, muxrpc.Method{"private", "publish"}, v, recpRefs)
	if err != nil {
		return refs.MessageRef{}, fmt.Errorf("ssbClient: private.publish call failed: %w", err)
	}
	return refs.ParseMessageRef(resp)
}

// PrivateRead returns a stream of private messages that can be read.
func (c Client) PrivateRead() (*muxrpc.ByteSource, error) {
	src, err := c.Source(c.rootCtx, muxrpc.TypeJSON, muxrpc.Method{"private", "read"})
	if err != nil {
		return nil, fmt.Errorf("ssbClient: private.read query failed: %w", err)
	}
	return src, nil
}

// CreateLogStream returns a source of all the messages in the bot.
func (c Client) CreateLogStream(o message.CreateLogArgs) (*muxrpc.ByteSource, error) {
	src, err := c.Source(c.rootCtx, muxrpc.TypeJSON, muxrpc.Method{"createLogStream"}, o)
	if err != nil {
		return nil, fmt.Errorf("ssbClient: failed to create stream: %w", err)
	}
	return src, nil
}

// CreateHistoryStream filters all the messages by author (ID in the argguments struct).
func (c Client) CreateHistoryStream(o message.CreateHistArgs) (*muxrpc.ByteSource, error) {
	src, err := c.Source(c.rootCtx, muxrpc.TypeJSON, muxrpc.Method{"createHistoryStream"}, o)
	if err != nil {
		return nil, fmt.Errorf("ssbClient: failed to create stream (%T): %w", o, err)
	}
	return src, nil
}

// MessagesByType filters all the messages by type
func (c Client) MessagesByType(o message.MessagesByTypeArgs) (*muxrpc.ByteSource, error) {
	src, err := c.Source(c.rootCtx, muxrpc.TypeJSON, muxrpc.Method{"messagesByType"}, o)
	if err != nil {
		return nil, fmt.Errorf("ssbClient: failed to create stream (%T): %w", o, err)
	}
	return src, nil
}

// TanglesThread returns the root message and replies to it
func (c Client) TanglesThread(o message.TanglesArgs) (*muxrpc.ByteSource, error) {
	src, err := c.Source(c.rootCtx, muxrpc.TypeJSON, muxrpc.Method{"tangles", "thread"}, o)
	if err != nil {
		return nil, fmt.Errorf("ssbClient/tangles: failed to create stream: %w", err)
	}
	return src, nil
}

// TODO: TanglesHeads

type noopHandler struct{ logger log.Logger }

func (noopHandler) Handled(m muxrpc.Method) bool { return false }

func (h noopHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}

func (h noopHandler) HandleCall(ctx context.Context, req *muxrpc.Request) {
	req.CloseWithError(fmt.Errorf("go-ssb/client: unsupported call"))
}
