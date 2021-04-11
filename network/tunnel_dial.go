// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"errors"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

type connectArg struct {
	Portal refs.FeedRef `json:"portal"`
	Target refs.FeedRef `json:"target"`
}

func (n *node) DialViaRoom(portal, target refs.FeedRef) error {
	portalLogger := kitlog.With(n.log, "portal", portal.ShortRef())

	edp, has := n.GetEndpointFor(portal)
	if !has {
		return errors.New("ssb/network: room offline")
	}

	var arg connectArg
	arg.Portal = portal
	arg.Target = target

	ctx := context.TODO()

	r, w, err := edp.Duplex(ctx, muxrpc.TypeBinary, muxrpc.Method{"tunnel", "connect"}, arg)
	if err != nil {
		return err
	}

	var tc tunnelConn
	tc.Reader = muxrpc.NewSourceReader(r)
	tc.WriteCloser = muxrpc.NewSinkWriter(w)
	tc.local = n.opts.ListenAddr

	tc.remote = tunnelHost{
		Host: portal,
	}

	authWrapper := n.secretClient.ConnWrapper(target.PubKey())

	conn, err := authWrapper(tc)
	if err != nil {
		level.Warn(portalLogger).Log("event", "tunnel.connect failed to authenticate", "err", err)
		return err
	}

	origin, err := ssb.GetFeedRefFromAddr(conn.RemoteAddr())
	if err != nil {
		level.Warn(portalLogger).Log("event", "failed to get feed for remote tunnel", "err", err)
		return err
	}

	level.Info(portalLogger).Log("event", "tunnel.connect established", "origin", origin.ShortRef())

	// start serving the connection
	go n.handleConnection(ctx, conn, false)

	return nil
}
