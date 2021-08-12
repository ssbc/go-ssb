// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.cryptoscope.co/muxrpc/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
)

func TestAutomaticUnboxing(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
	)
	r.NoError(err, "sbot srv init failed")

	srv.PublishLog.Publish(map[string]string{"type": "test", "hello": "world"})

	c, err := client.NewUnix(filepath.Join(srvRepo, "socket"))
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	ref, err := c.Whoami()
	r.NoError(err, "failed to call whoami")
	r.NotNil(ref)
	a.True(srv.KeyPair.ID().Equal(ref), "whoami not equal")

	groupID, root, err := srv.Groups.Create("client test group")
	r.NoError(err, "failed to create group")

	hello1, err := srv.Groups.PublishPostTo(groupID, "hello 1!")
	r.NoError(err, "failed to post hello1")

	hello2, err := srv.Groups.PublishPostTo(groupID, "hello 2!")
	r.NoError(err, "failed to post hello2")

	args := message.MessagesByTypeArgs{Type: "post"}
	args.Private = false
	args.Limit = -1
	src, err := c.MessagesByType(args)
	r.NoError(err, "failed to create source for public messages")
	testElementsInSource(t, src, 0)

	args.Private = true
	src, err = c.MessagesByType(args)
	r.NoError(err, "failed to create source for private messages")
	testElementsInSource(t, src, 2)

	// TODO: check messages hello1 and hello2 are included in src
	_ = hello1
	_ = hello2

	targs := message.TanglesArgs{
		Root: root,
		Name: "group",
	}
	targs.Limit = -1
	targs.Private = false
	src, err = c.TanglesThread(targs)
	r.NoError(err, "failed to create source for tangled messages (public)")
	testElementsInSource(t, src, 0)

	targs.Private = true
	src, err = c.TanglesThread(targs)
	r.NoError(err, "failed to create source for tangled messages (private)")
	testElementsInSource(t, src, 3) // add-member + the two posts

	// cleanup
	c.Terminate()

	srv.Shutdown()
	srv.Close()
}

func testElementsInSource(t *testing.T, src *muxrpc.ByteSource, cnt int) {
	ctx := context.Background()
	r, a := require.New(t), assert.New(t)

	i := 0

	for src.Next(ctx) {

		body, err := src.Bytes()
		r.NoError(err)
		t.Log(string(body))
		_ = body

		i++
		if i > cnt {
			t.Error("testElementsInSource: way to many results")
		}
	}

	err := src.Err()
	r.NoError(err, "failed to get all elements from source (public)")
	a.Equal(cnt, i)
}
