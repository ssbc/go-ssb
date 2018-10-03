package tests

import (
	"bytes"
	"context"
	"io/ioutil"
	"os/exec"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/sbot/sbot"
)

func initInterop(t *testing.T, fname string) {
	r := require.New(t)
	ctx := context.Background()

	dir, err := ioutil.TempDir("", t.Name())
	r.NoError(err, "failed to create testdir for repo")

	sbot, err := sbot.New(
		sbot.WithListenAddr("localhost:0"),
		sbot.WithRepoPath(dir),
		sbot.WithContext(ctx),
	)
	r.NoError(err, "failed to init test go-sbot")
	t.Logf("go-sbot: %s", sbot.KeyPair.Id.Ref())

	go func() {
		err := sbot.Node.Serve(ctx)
		r.NoError(err, "serving test go-sbot exited")
	}()
	b := new(bytes.Buffer)
	cmd := exec.Command("node", "./sbot.js")
	cmd.Stderr = logtest.Logger(t.Name(), t)
	cmd.Stdout = b
	cmd.Env = []string{
		"TEST_NAME=" + t.Name(),
		"TEST_BOB=" + sbot.KeyPair.Id.Ref(),
		"TEST_GOADDR=" + netwrap.GetAddr(sbot.Node.GetListenAddr(), "tcp").String(),
		"TEST_ACTIONS=" + fname,
	}

	r.NoError(cmd.Run(), "failed to init test js-sbot")
	t.Logf("JSbot: %s", b.String())

	r.NoError(sbot.Close(), "failed to close go-sbot")
}

func TestInteropFeeds(t *testing.T) {
	initInterop(t, "/tmp/foo.js")
}
