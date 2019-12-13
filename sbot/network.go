package sbot

import (
	"context"
	"errors"
	"net"

	"go.cryptoscope.co/ssb"
)

func (s *Sbot) Connect(ctx context.Context, remote net.Addr) error {
	if remote == nil {
		return errors.New("sbot/connect: no remote address")
	}
	for _, n := range s.networks {
		if n.HasScope(ssb.NetworkScopePublic) {
			return n.Connect(ctx, remote)
		}
	}
	return errors.New("sbot/connect: no public scope network for dialing out")
}

func (s *Sbot) GetListenAddr(sc ssb.NetworkScope) net.Addr {
	for _, n := range s.networks {
		if n.HasScope(sc) {
			return n.GetListenAddr()
		}
	}
	return nil
}

func (s *Sbot) GetConnTracker(scope ssb.NetworkScope) ssb.ConnTracker {
	return nil
}

type scopedNetwork struct {
	ssb.Network
	Scope ssb.NetworkScope
}
