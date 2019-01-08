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

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
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

// returns the created go-sbot, the pubkey of the jsbot, a wait and a cleanup function
func initInterop(t *testing.T, jsbefore, jsafter string, sbotOpts ...sbot.Option) (*sbot.Sbot, *ssb.FeedRef, <-chan bool, <-chan error, func()) {
	r := require.New(t)
	ctx := context.Background()

	dir, err := ioutil.TempDir("", t.Name())
	r.NoError(err, "failed to create testdir for repo")

	// Choose you logger!
	// use the "logtest" line if you want to log through calls to `t.Log`
	// use the "NewLogfmtLogger" line if you want to log to stdout
	// the test logger does not print anything if the command hangs, so you have an alternative
	var info logging.Interface
	if testing.Verbose() {
		// TODO: multiwriter
		info = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	} else {
		info, _ = logtest.KitLogger("go", t)
	}
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

	var errc = make(chan error, 1)
	go func() {
		err := sbot.Node.Serve(ctx)
		if err != nil {
			errc <- errors.Wrap(err, "node serve exited")
		}
		close(errc)
	}()

	alice, done, nodeErrc := startJSBot(t,
		jsbefore,
		jsafter,
		sbot.KeyPair.Id.Ref(),
		netwrap.GetAddr(sbot.Node.GetListenAddr(), "tcp").String())

	return sbot, alice, done, mergeErrorChans(nodeErrc, errc), func() {
		<-done
		if !t.Failed() {
			r.NoError(os.RemoveAll(dir), "error removing test directory")
		}
	}
}

// returns the jsbots pubkey, a wait func and a done channel
func startJSBot(t *testing.T, jsbefore, jsafter, goRef, goAddr string) (*ssb.FeedRef, <-chan bool, <-chan error) {
	r := require.New(t)
	cmd := exec.Command("node", "./sbot.js")
	if testing.Verbose() {
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = logtest.Logger("js", t)
	}
	outrc, err := cmd.StdoutPipe()
	r.NoError(err)

	cmd.Env = []string{
		"TEST_NAME=" + t.Name(),
		"TEST_BOB=" + goRef,
		"TEST_GOADDR=" + goAddr,
		"TEST_BEFORE=" + writeFile(t, jsbefore),
		"TEST_AFTER=" + writeFile(t, jsafter),
	}

	r.NoError(cmd.Start(), "failed to init test js-sbot")

	var done = make(chan bool)
	var errc = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			errc <- errors.Wrap(err, "cmd wait failed")
		}
		t.Log("waited")
		close(done)
		close(errc)
	}()

	pubScanner := bufio.NewScanner(outrc) // TODO muxrpc comms?
	r.True(pubScanner.Scan(), "multiple lines of output from js - expected #1 to be alices pubkey/id")

	alice, err := ssb.ParseFeedRef(pubScanner.Text())
	r.NoError(err, "failed to get alice key from JS process")
	t.Logf("JS alice: %s", alice.Ref())
	return alice, done, errc
}

// utils
func mergeErrorChans(cs ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error, 1)

	output := func(c <-chan error) {
		for a := range c {
			out <- a
		}
		wg.Done()
	}

	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
