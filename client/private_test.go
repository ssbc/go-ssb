package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.cryptoscope.co/luigi"

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
	a.Equal(srv.KeyPair.Id.Ref(), ref.Ref())

	groupID, root, err := srv.Groups.Create("client test group")
	r.NoError(err, "failed to create group")

	hello1, err := srv.Groups.PublishPostTo(groupID, "hello 1!")
	r.NoError(err, "failed to post hello1")

	hello2, err := srv.Groups.PublishPostTo(groupID, "hello 2!")
	r.NoError(err, "failed to post hello2")

	args := message.MessagesByTypeArgs{Type: "post"}
	args.Private = false
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
	targs.Private = false
	src, err = c.Tangles(targs)
	r.NoError(err, "failed to create source for tangled messages (public)")
	testElementsInSource(t, src, 0)

	targs.Private = true
	src, err = c.Tangles(targs)
	r.NoError(err, "failed to create source for tangled messages (private)")
	testElementsInSource(t, src, 3) // add-member + the two posts
}

func testElementsInSource(t *testing.T, src luigi.Source, cnt int) {
	ctx := context.Background()
	r, a := require.New(t), assert.New(t)
	var elems []interface{}
	var snk = luigi.NewSliceSink(&elems)
	err := luigi.Pump(ctx, snk, src)
	r.NoError(err, "failed to get all elements from source (public)")
	a.Len(elems, cnt)
}
