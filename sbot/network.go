package sbot

import (
	"context"
	"errors"
	"net"

	"go.cryptoscope.co/ssb"

	"go.cryptoscope.co/ssb/network"
)

func (s *Sbot) Connect(ctx context.Context, remote net.Addr) error {
	if remote == nil {
		return errors.New("sbot/connect: no remote address")
	}
	return errors.New("TODO:connect")
}

func (s *Sbot) GetListenAddr(scope network.Scope) net.Addr {
	return nil
}

func (s *Sbot) GetConnTracker(scope network.Scope) ssb.ConnTracker {
	return nil
}
