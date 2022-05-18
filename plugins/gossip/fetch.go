// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package gossip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/ssbc/go-muxrpc/v2"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"
	"golang.org/x/sync/errgroup"

	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb/internal/neterr"
	"github.com/ssbc/go-ssb/message"
)

const limit = 1000

func (h *LegacyGossip) FetchAll(
	ctx context.Context,
	edp muxrpc.Endpoint,
	withLive bool,
) error {
	set := h.WantList.ReplicationList()

	feeds, err := set.List()
	if err != nil {
		return err
	}

	rand.Shuffle(len(feeds), func(i, j int) {
		feeds[i], feeds[j] = feeds[j], feeds[i]
	})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	feedCh := make(chan refs.FeedRef)

	errGroup := h.startWorkers(ctx, feedCh, edp)

	go func() {
		defer close(feedCh)

		for _, feed := range feeds {
			select {
			case <-ctx.Done():
				return
			case feedCh <- feed:
				continue
			}
		}
	}()

	return errGroup.Wait()
}

func (h *LegacyGossip) startWorkers(ctx context.Context, feedCh <-chan refs.FeedRef, edp muxrpc.Endpoint) *errgroup.Group {
	errGroup, ctx := errgroup.WithContext(ctx)

	for i := 0; i < h.numberOfConcurrentReplicationsPerPeer; i++ {
		errGroup.Go(
			func() error {
				for {
					select {
					case feed, ok := <-feedCh:
						if !ok {
							return nil
						}

						if err := h.workFeed(ctx, edp, feed, false); err != nil {
							return err
						}
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			},
		)
	}

	return errGroup
}

func (h *LegacyGossip) workFeed(ctx context.Context, edp muxrpc.Endpoint, ref refs.FeedRef, withLive bool) error {
	select {
	case <-h.tokenPool.GetToken():
		defer h.tokenPool.ReturnToken()
	case <-ctx.Done():
		return ctx.Err()
	}

	onComplete, shouldReplicate := h.feedTracker.TryReplicate(edp.Remote(), ref)
	if !shouldReplicate {
		return nil
	}

	fetchedMessages, err := h.fetchFeed(ctx, ref, edp, time.Now(), withLive)
	var callErr *muxrpc.CallError
	var noSuchMethodErr muxrpc.ErrNoSuchMethod
	if muxrpc.IsSinkClosed(err) ||
		errors.As(err, &noSuchMethodErr) ||
		errors.As(err, &callErr) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, muxrpc.ErrSessionTerminated) ||
		neterr.IsConnBrokenErr(err) {
		return err
	} else if err != nil {
		// just logging the error assuming forked feed for instance
		level.Warn(h.Info).Log("event", "skipped updating of stored feed", "err", err, "fr", ref.ShortSigil())
	}

	if fetchedMessages < limit {
		onComplete(ReplicationResultDoesNotHaveMoreMessages)
	}

	return nil
}

// fetchFeed requests the feed fr from endpoint e into the repo of the handler
func (h *LegacyGossip) fetchFeed(
	ctx context.Context,
	fr refs.FeedRef,
	edp muxrpc.Endpoint,
	started time.Time,
	withLive bool,
) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	// wether to fetch this feed in full (TODO: should be passed in)
	completeFeed := true
	var err error
	snk, err := h.verifyRouter.GetSink(fr, completeFeed)
	if err != nil {
		return 0, fmt.Errorf("failed to get verify sink for feed: %w", err)
	}

	var latestSeq = int(snk.Seq())
	startSeq := latestSeq
	info := log.With(h.Info, "event", "gossiprx",
		"fr", fr.ShortSigil(),
		"starting", latestSeq) // , "me", g.Id.ShortRef())

	var q = message.NewCreateHistoryStreamArgs()
	q.ID = fr
	q.Seq = int64(latestSeq + 1)
	q.Live = withLive
	q.Limit = limit

	defer func() {
		if n := latestSeq - startSeq; n > 0 {
			if h.sysGauge != nil {
				h.sysGauge.With("part", "msgs").Add(float64(n))
			}
			if h.sysCtr != nil {
				h.sysCtr.With("event", "gossiprx").Add(float64(n))
			}
			level.Debug(info).Log("received", n, "took", time.Since(started))
		}
	}()

	// level.Info(info).Log("starting", "fetch")
	method := muxrpc.Method{"createHistoryStream"}

	var src *muxrpc.ByteSource
	switch fr.Algo() {
	case refs.RefAlgoFeedSSB1:
		src, err = edp.Source(ctx, muxrpc.TypeJSON, method, q)
	case refs.RefAlgoFeedBendyButt:
		fallthrough
	case refs.RefAlgoFeedGabby:
		src, err = edp.Source(ctx, muxrpc.TypeBinary, method, q)
	default:
		return 0, fmt.Errorf("fetchFeed(%s): unhandled feed format", fr.String())
	}
	if err != nil {
		return 0, fmt.Errorf("fetchFeed(%s:%d) failed to create source: %w", fr.String(), latestSeq, err)
	}

	counter := 0

	var buf = &bytes.Buffer{}
	for src.Next(ctx) {
		err = src.Reader(func(r io.Reader) error {
			_, err = buf.ReadFrom(r)
			return err
		})
		if err != nil {
			return counter, err
		}

		err = snk.Verify(buf.Bytes())
		if err != nil {
			return counter, err
		}
		buf.Reset()

		latestSeq++
		counter++
	}

	if err := src.Err(); err != nil {
		return counter, fmt.Errorf("fetchFeed(%s:%d) gossip pump failed: %w", fr.String(), latestSeq, err)
	}

	return counter, nil
}

type TokenPool struct {
	ch chan struct{}
}

func NewTokenPool(n int) *TokenPool {
	ch := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		ch <- struct{}{}
	}

	return &TokenPool{
		ch: ch,
	}
}

func (p *TokenPool) GetToken() <-chan struct{} {
	return p.ch
}

func (p *TokenPool) ReturnToken() {
	p.ch <- struct{}{}
}
