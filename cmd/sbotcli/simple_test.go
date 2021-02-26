package main_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func buildCLI(t *testing.T) string {
	cliPath := filepath.Join("testrun", t.Name(), "sbotcli-test")
	err := exec.Command("go", "build", "-race", "-o", cliPath).Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Remove(cliPath)
	})
	return cliPath
}

// returns a func thay you can pass CLI arguments to and returns the output, which is mirroed to stderr for assertions.
// TODO: sepearte stderr and stdout for better output assertions
func mkCommandRunner(t *testing.T, ctx context.Context, path string, sockPath string) func(...string) []byte {
	var out bytes.Buffer

	return func(args ...string) []byte {
		out.Reset()

		argsWithSockPath := append([]string{"--unixsock", sockPath}, args...)

		sbotcli := exec.CommandContext(ctx, path, argsWithSockPath...)
		sbotcli.Stdout = io.MultiWriter(os.Stderr, &out)
		sbotcli.Stderr = io.MultiWriter(os.Stderr, &out)

		err := sbotcli.Run()
		if err != nil {
			t.Error(err)
		}

		return out.Bytes()
	}
}

func TestWhoami(t *testing.T) {
	cliPath := buildCLI(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithContext(ctx),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
	)
	r.NoError(err, "sbot srv init failed")

	go func() {
		err = srv.Network.Serve(ctx)
		r.NoError(err)
	}()

	sbotcli := mkCommandRunner(t, ctx, cliPath, filepath.Join(srvRepo, "socket"))

	out := sbotcli("call", "whoami")

	has := bytes.Contains(out, []byte(srv.KeyPair.Id.Ref()))
	a.True(has, "ID not found in output")

	srv.Shutdown()
	err = srv.Close()
	r.NoError(err)
}

func TestPublish(t *testing.T) {
	cliPath := buildCLI(t)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)

	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(os.Stderr)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithContext(ctx),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
	)
	r.NoError(err, "sbot srv init failed")

	go func() {
		err = srv.Network.Serve(ctx)
		r.NoError(err)
	}()

	v, err := srv.ReceiveLog.Seq().Value()
	r.NoError(err)
	a.EqualValues(-1, v.(margaret.Seq).Seq(), "log not empty")

	sbotcli := mkCommandRunner(t, ctx, cliPath, filepath.Join(srvRepo, "socket"))

	out := sbotcli("publish", "post", "hell, world!")

	has := bytes.Contains(out, []byte(".sha256"))
	a.True(has, "has a message hash")

	v, err = srv.ReceiveLog.Seq().Value()
	r.NoError(err)
	a.EqualValues(0, v.(margaret.Seq).Seq(), "first message")

	theFeed := &refs.FeedRef{
		ID:   bytes.Repeat([]byte{1}, 32),
		Algo: refs.RefAlgoFeedSSB1,
	}
	out = sbotcli("publish", "contact", "--following", theFeed.Ref())

	has = bytes.Contains(out, []byte(".sha256"))
	a.True(has, "has a message hash")

	v, err = srv.ReceiveLog.Seq().Value()
	r.NoError(err)
	a.EqualValues(1, v.(margaret.Seq).Seq(), "2nd message")

	srv.Shutdown()
	err = srv.Close()
	r.NoError(err)
}
