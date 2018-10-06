package tests

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/netwrap"
	ssb "go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/sbot"
)

func writeFile(t *testing.T, data string) string {
	r := require.New(t)
	f, err := ioutil.TempFile("", t.Name())
	r.NoError(err)
	_, err = fmt.Fprintf(f, "%s", data)
	r.NoError(err)
	err = f.Close()
	r.NoError(err)
	return f.Name()
}

func initInterop(t *testing.T, jsbefore, jsafter string, sbotOpts ...sbot.Option) (*sbot.Sbot, *ssb.FeedRef, <-chan struct{}) {
	r := require.New(t)
	ctx := context.Background()

	exited := make(chan struct{})
	dir, err := ioutil.TempDir("", t.Name())
	r.NoError(err, "failed to create testdir for repo")

	// Choose you logger!
	// use the "logtest" line if you want to log through calls to `t.Log`
	// use the "NewLogfmtLogger" line if you want to log to stdout
	// the test logger does not print anything if the command hangs, so you have an alternative
	info, _ := logtest.KitLogger("go", t)
	//info := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))

	// timestamps!
	info = log.With(info, "ts", log.TimestampFormat(time.Now, "3:04:05.000"))

	// prepend defaults
	sbotOpts = append([]sbot.Option{
		sbot.WithInfo(info),
		sbot.WithListenAddr("localhost:0"),
		sbot.WithRepoPath(dir),
		sbot.WithContext(ctx),
	}, sbotOpts...)

	sbot, err := sbot.New(sbotOpts...)
	r.NoError(err, "failed to init test go-sbot")
	t.Logf("go-sbot: %s", sbot.KeyPair.Id.Ref())

	go func() {
		err := sbot.Node.Serve(ctx)
		r.NoError(err, "serving test go-sbot exited")
	}()
	pr, pw := io.Pipe()
	cmd := exec.Command("node", "./sbot.js")
	cmd.Stderr = logtest.Logger("js", t)
	cmd.Stdout = pw
	cmd.Env = []string{
		"TEST_NAME=" + t.Name(),
		"TEST_BOB=" + sbot.KeyPair.Id.Ref(),
		"TEST_GOADDR=" + netwrap.GetAddr(sbot.Node.GetListenAddr(), "tcp").String(),
		"TEST_BEFORE=" + writeFile(t, jsbefore),
		"TEST_AFTER=" + writeFile(t, jsafter),
	}

	r.NoError(cmd.Start(), "failed to init test js-sbot")

	go func() {
		err := cmd.Wait()
		r.NoError(err, "js-sbot exited")
		close(exited)
	}()

	pubScanner := bufio.NewScanner(pr) // TODO muxrpc comms?
	r.True(pubScanner.Scan(), "multiple lines of output from js - expected #1 to be alices pubkey/id")

	alice, err := ssb.ParseFeedRef(pubScanner.Text())
	r.NoError(err, "failed to get alice key from JS process")
	t.Logf("JS alice: %s", alice.Ref())

	return sbot, alice, exited
}
