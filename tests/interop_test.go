package tests

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/sbot"
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

func initInterop(t *testing.T, jsbefore, jsafter string, sbotOpts ...sbot.Option) (*sbot.Sbot, *ssb.FeedRef, func() func()) {
	t.Parallel()
	r := require.New(t)
	ctx := context.Background()

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
		if err != nil {
			fmt.Fprintln(os.Stderr, "warn: go-sbot muxrpc exited", err)
			// t.Fatal(err) BUG?!
			os.Exit(42)
		}
	}()

	cmd := exec.Command("node", "./sbot.js")
	cmd.Stderr = logtest.Logger("js", t)
	outrc, err := cmd.StdoutPipe()
	r.NoError(err)

	cmd.Env = []string{
		"TEST_NAME=" + t.Name(),
		"TEST_BOB=" + sbot.KeyPair.Id.Ref(),
		"TEST_GOADDR=" + netwrap.GetAddr(sbot.Node.GetListenAddr(), "tcp").String(),
		"TEST_BEFORE=" + writeFile(t, jsbefore),
		"TEST_AFTER=" + writeFile(t, jsafter),
	}

	r.NoError(cmd.Start(), "failed to init test js-sbot")

	var o sync.Once
	cleanup := func() func() {
		o.Do(func() {
			err := cmd.Wait()
			r.NoError(err, "js-sbot exited")
		})

		return func() {
			if !t.Failed() {
				r.NoError(os.RemoveAll(dir), "error removing test directory")
			}
		}
	}

	pubScanner := bufio.NewScanner(outrc) // TODO muxrpc comms?
	r.True(pubScanner.Scan(), "multiple lines of output from js - expected #1 to be alices pubkey/id")

	alice, err := ssb.ParseFeedRef(pubScanner.Text())
	r.NoError(err, "failed to get alice key from JS process")
	t.Logf("JS alice: %s", alice.Ref())

	return sbot, alice, cleanup
}
