package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/message"
	"go.cryptoscope.co/secretstream"
	"github.com/pkg/errors"
)

type gossip struct {
	I    sbot.FeedRef
	Node sbot.Node
	Repo sbot.Repo

	// ugly hack
	running sync.Mutex
}

func (g *gossip) fetchFeed(ctx context.Context, fr *sbot.FeedRef, e muxrpc.Endpoint) error {
	latestIdxKey := librarian.Addr(fmt.Sprintf("latest:%s", fr.Ref()))
	idx := g.Repo.GossipIndex()
	latestObv, err := idx.Get(ctx, latestIdxKey)
	if err != nil {
		return errors.Wrapf(err, "idx latest failed")
	}
	latest, err := latestObv.Value()
	if err != nil {
		return errors.Wrapf(err, "failed to observe latest")
	}

	var latestSeq margaret.Seq
	switch v := latest.(type) {
	case librarian.UnsetValue:
	case margaret.Seq:
		latestSeq = v
	}

	var q = message.CreateHistArgs{false, false, fr.Ref(), latestSeq + 1, 100}
	source, err := e.Source(ctx, message.RawSignedMessage{}, []string{"createHistoryStream"}, q)
	if err != nil {
		return errors.Wrapf(err, "createHistoryStream failed")
	}

	var more bool
	start := time.Now()
	for {
		v, err := source.Next(ctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "failed to drain createHistoryStream logSeq:%d", latestSeq)
		}

		rmsg := v.(message.RawSignedMessage)

		ref, dmsg, err := message.Verify(rmsg.RawMessage)
		if err != nil {
			return errors.Wrap(err, "simple Encode failed")
		}
		//log.Log("event", "got", "hist", dmsg.Sequence, "ref", ref.Ref())

		// todo: check previous etc.. maybe we want a mapping sink here
		_, err = g.Repo.Log().Append(message.StoredMessage{
			Author:    dmsg.Author,
			Previous:  dmsg.Previous,
			Key:       *ref,
			Sequence:  dmsg.Sequence,
			Timestamp: time.Now(),
			Raw:       rmsg.RawMessage,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to append message (%s) Seq:%d", ref.Ref(), dmsg.Sequence)
		}
		more = true
		latestSeq = dmsg.Sequence
	} // hist drained

	if !more {
		return nil
	}

	if err := idx.Set(ctx, latestIdxKey, latestSeq); err != nil {
		return errors.Wrapf(err, "failed to update sequence for author %s", fr.Ref())
	}

	f := func(ctx context.Context, seq margaret.Seq, v interface{}, idx librarian.SetterIndex) error {
		smsg, ok := v.(message.StoredMessage)
		if !ok {
			return errors.Errorf("unexpected type: %T - wanted storedMsg", v)
		}
		addr := fmt.Sprintf("%s:%06d", smsg.Author.Ref(), smsg.Sequence)
		err := idx.Set(ctx, librarian.Addr(addr), seq)
		return errors.Wrapf(err, "failed to update idx for k:%s - v:%d", addr, seq)
	}
	sinkIdx := librarian.NewSinkIndex(f, idx)

	src, err := g.Repo.Log().Query(sinkIdx.QuerySpec())
	if err != nil {
		return errors.Wrapf(err, "failed to construct index update query")
	}
	if err := luigi.Pump(ctx, sinkIdx, src); err != nil {
		return errors.Wrap(err, "error pumping from queried src to SinkIndex")
	}

	log.Log("event", "verfied", "latest", latestSeq, "feedref", fr.Ref(), "took", time.Since(start))
	return nil
}

func (c *gossip) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	c.running.Lock()
	defer c.running.Unlock()

	srv := e.(muxrpc.Server)
	log.Log("event", "onConnect", "handler", "gossip", "addr", srv.Remote())
	shsID := netwrap.GetAddr(srv.Remote(), "shs-bs").(secretstream.Addr)
	ref, err := sbot.ParseRef(shsID.String())
	if err != nil {
		log.Log("handleConnect", "sbot.ParseRef", "err", err)
		return
	}

	fref, ok := ref.(*sbot.FeedRef)
	if !ok {
		log.Log("handleConnect", "notFeedRef", "r", shsID.String())
		return
	}

	if err := c.fetchFeed(ctx, fref, e); err != nil {
		log.Log("handleConnect", "fetchFeed failed", "r", fref.Ref(), "err", err)
		return
	}

	if err := c.fetchFeed(ctx, &c.I, e); err != nil {
		log.Log("handleConnect", "fetchFeed failed", "r", c.I.Ref(), "err", err)
		return
	}

	kf, err := c.Repo.KnownFeeds()
	if err != nil {
		log.Log("handleConnect", "knownFeeds failed", "err", err)
		return
	}
	/* lame init
	kf["@f/6sQ6d2CMxRUhLpspgGIulDxDCwYD7DzFzPNr7u5AU=.ed25519"] = 0
	kf["@EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=.ed25519"] = 0
	kf["@YXkE3TikkY4GFMX3lzXUllRkNTbj5E+604AkaO1xbz8=.ed25519"] = 0
	*/

	for feed, _ := range kf {
		ref, err := sbot.ParseRef(feed)
		if err != nil {
			log.Log("handleConnect", "ParseRef failed", "err", err)
			return
		}

		fref, ok = ref.(*sbot.FeedRef)
		if !ok {
			log.Log("handleConnect", "caset failed", "type", fmt.Sprintf("%T", ref))
			return
		}

		err = c.fetchFeed(ctx, fref, e)
		if err != nil {
			log.Log("handleConnect", "knownFeeds failed", "err", err)
			return
		}
	}
}

func (c *gossip) HandleCall(ctx context.Context, req *muxrpc.Request) {
	log.Log("event", "onCall", "handler", "gossip", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)

	var closed bool
	checkAndClose := func(err error) {
		checkAndLog(err)
		if err != nil {
			closed = true
			closeErr := req.Stream.CloseWithError(err)
			checkAndLog(errors.Wrapf(closeErr, "error closeing request. %s", req.Method))
		}
	}

	defer func() {
		if !closed {
			checkAndLog(errors.Wrapf(req.Stream.Close(), "gossip: error closing call: %s", req.Method))
		}
	}()

	switch req.Method.String() {
	case "gossip.ping":
		if err := c.ping(ctx, req); err != nil {
			checkAndClose(errors.Wrap(err, "gossip.ping failed."))
			return
		}

	case "gossip.connect":
		if len(req.Args) != 1 {
			// TODO: use secretstream
			log.Log("error", "usage", "args", req.Args, "method", req.Method)
			checkAndClose(errors.New("usage: gossip.connect host:port:key"))
			return
		}

		destString, ok := req.Args[0].(string)
		if !ok {
			err := errors.Errorf("gossip.connect call: expected argument to be string, got %T", req.Args[0])
			checkAndClose(err)
			return
		}

		if err := c.connect(ctx, destString); err != nil {
			checkAndClose(errors.Wrap(err, "gossip.connect failed."))
			return
		}
	default:
		checkAndClose(errors.Errorf("unknown command: %s", req.Method))
	}
}

func (g *gossip) ping(ctx context.Context, req *muxrpc.Request) error {
	log.Log("event", "ping", "args", fmt.Sprintf("%v", req.Args))
	err := req.Stream.Pour(ctx, time.Now().Unix())
	return errors.Wrap(err, "failed to pour ping")
}

func (g *gossip) connect(ctx context.Context, dest string) error {
	splitted := strings.Split(dest, ":")
	if n := len(splitted); n != 3 {
		return errors.Errorf("gossip.connect: bad request. expected 3 parts, got %d", n)
	}

	addr, err := net.ResolveTCPAddr("tcp", strings.Join(splitted[:2], ":"))
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: error resolving network address %q", splitted[:2])
	}

	ref, err := sbot.ParseRef(splitted[2])
	if err != nil {
		return errors.Wrapf(err, "gossip.connect call: failed to parse FeedRef %s", splitted[2])
	}

	remoteFeed, ok := ref.(*sbot.FeedRef)
	if !ok {
		return errors.Errorf("gossip.connect: expected FeedRef got %T", ref)
	}

	wrappedAddr := netwrap.WrapAddr(addr, secretstream.Addr{PubKey: remoteFeed.ID})
	err = g.Node.Connect(ctx, wrappedAddr)
	return errors.Wrapf(err, "gossip.connect call: error connecting to %q", addr)
}
