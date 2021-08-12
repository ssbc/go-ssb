// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package network_test

import (
	"context"
	"crypto/rand"
	"net"
	"os"
	"testing"

	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/network"
)

func TestIsServer(t *testing.T) {
	r := require.New(t)

	ctx := context.Background()

	var appkey = make([]byte, 32)
	rand.Read(appkey)

	logger := log.NewLogfmtLogger(os.Stderr)

	kpClient, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	kpServ, err := ssb.NewKeyPair(nil, refs.RefAlgoFeedSSB1)
	r.NoError(err)

	client, err := network.New(network.Options{
		Logger:  logger,
		AppKey:  appkey,
		KeyPair: kpClient,

		MakeHandler: makeServerHandler(t, true),
	})
	r.NoError(err)

	server, err := network.New(network.Options{
		Logger:  logger,
		AppKey:  appkey,
		KeyPair: kpServ,

		ListenAddr: &net.TCPAddr{Port: 0}, // any random port

		MakeHandler: makeServerHandler(t, false),
	})
	r.NoError(err)

	go func() {
		err = server.Serve(ctx)
		if err != nil {
			panic(err)
		}
	}()

	err = client.Connect(ctx, server.GetListenAddr())
	r.NoError(err)

	client.Close()
	server.Close()
}

type testHandler struct {
	t          *testing.T
	wantServer bool
}

func (th testHandler) Handled(muxrpc.Method) bool { return true }

func (th testHandler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {
	require.Equal(th.t, th.wantServer, muxrpc.IsServer(e), "server assertion failed")
}

func (th testHandler) HandleCall(ctx context.Context, req *muxrpc.Request) {}

func makeServerHandler(t *testing.T, wantServer bool) func(net.Conn) (muxrpc.Handler, error) {
	return func(_ net.Conn) (muxrpc.Handler, error) {
		return testHandler{t, wantServer}, nil
	}
}
