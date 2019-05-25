package tests

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/debug"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/sbot"
)

func init() {
	err := os.RemoveAll("testrun")
	if err != nil {
		fmt.Println("failed to clean testrun dir")
		panic(err)
	}
}

func writeFile(t *testing.T, data string) string {
	r := require.New(t)
	f, err := ioutil.TempFile("testrun/"+t.Name(), "*.js")
	r.NoError(err)
	_, err = fmt.Fprintf(f, "%s", data)
	r.NoError(err)
	err = f.Close()
	r.NoError(err)
	return f.Name()
}

type testSession struct {
	t *testing.T

	keySHS, keyHMAC []byte

	// since we can't pass *testing.T to other goroutines, we use this to collect errors from background taskts
	backgroundErrs []<-chan error

	gobot *sbot.Sbot

	doneJS, doneGo <-chan struct{}
}

// TODO: restrucuture so that we can test both (default and random net keys) with the same Code

// rolls random values for secret-handshake app-key and HMAC
func newRandomSession(t *testing.T) *testSession {
	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)
	return newSession(t, appKey, hmacKey)
}

// if appKey is nil, the default value is used
// if hmac is nil, the object string is signed instead
func newSession(t *testing.T, appKey, hmacKey []byte) *testSession {
	return &testSession{
		t:       t,
		keySHS:  appKey,
		keyHMAC: hmacKey,
	}
}

// returns the created go-sbot, the pubkey of the jsbot, a wait and a cleanup function
func (ts *testSession) startGoBot(sbotOpts ...sbot.Option) {
	r := require.New(ts.t)
	ctx := context.Background()

	dir := filepath.Join("testrun", ts.t.Name())
	os.RemoveAll(dir)

	// Choose you logger!
	// use the "logtest" line if you want to log through calls to `t.Log`
	// use the "NewLogfmtLogger" line if you want to log to stdout
	// the test logger does not print anything if the command hangs, so you have an alternative
	var info logging.Interface
	if testing.Verbose() {
		// TODO: multiwriter
		info = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	} else {
		info, _ = logtest.KitLogger("go", ts.t)
	}
	// timestamps!
	info = log.With(info, "ts", log.TimestampFormat(time.Now, "3:04:05.000"))

	// prepend defaults
	sbotOpts = append([]sbot.Option{
		sbot.WithInfo(info),
		sbot.WithListenAddr("localhost:0"),
		sbot.WithRepoPath(dir),
		sbot.WithContext(ctx),
		sbot.WithConnWrapper(func(conn net.Conn) (net.Conn, error) {
			return debug.WrapConn(info, conn), nil
		}),
	}, sbotOpts...)

	if ts.keySHS != nil {
		sbotOpts = append(sbotOpts, sbot.WithAppKey(ts.keySHS))
	}
	if ts.keyHMAC != nil {
		sbotOpts = append(sbotOpts, sbot.WithHMACSigning(ts.keyHMAC))
	}

	sbot, err := sbot.New(sbotOpts...)
	r.NoError(err, "failed to init test go-sbot")
	ts.t.Logf("go-sbot: %s", sbot.KeyPair.Id.Ref())

	var done = make(chan struct{})
	var errc = make(chan error, 1)
	go func() {
		err := sbot.Network.Serve(ctx)
		if err != nil {
			errc <- errors.Wrap(err, "node serve exited")
		}
		close(done)
		close(errc)
	}()
	ts.doneGo = done
	ts.backgroundErrs = append(ts.backgroundErrs, errc)
	ts.gobot = sbot

	// TODO: make muxrpc client and connect to whoami for _ready_ ?
	return
}

// returns the jsbots pubkey
func (ts *testSession) startJSBot(jsbefore, jsafter string) *ssb.FeedRef {
	r := require.New(ts.t)
	cmd := exec.Command("node", "./sbot.js")
	if testing.Verbose() {
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = logtest.Logger("js", ts.t)
	}
	outrc, err := cmd.StdoutPipe()
	r.NoError(err)

	env := []string{
		"TEST_NAME=" + ts.t.Name(),
		"TEST_BOB=" + ts.gobot.KeyPair.Id.Ref(),
		"TEST_GOADDR=" + netwrap.GetAddr(ts.gobot.Network.GetListenAddr(), "tcp").String(),
		"TEST_BEFORE=" + writeFile(ts.t, jsbefore),
		"TEST_AFTER=" + writeFile(ts.t, jsafter),
	}
	if ts.keySHS != nil {
		env = append(env, "TEST_APPKEY="+base64.StdEncoding.EncodeToString(ts.keySHS))
	}

	if ts.keyHMAC != nil {
		env = append(env, "TEST_HMACKEY="+base64.StdEncoding.EncodeToString(ts.keyHMAC))
	}
	cmd.Env = env
	r.NoError(cmd.Start(), "failed to init test js-sbot")

	var done = make(chan struct{})
	var errc = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			errc <- errors.Wrap(err, "cmd wait failed")
		}
		close(done)
		fmt.Fprintf(os.Stderr, "\nJS Sbot process returned\n")
		close(errc)
	}()
	ts.doneJS = done
	ts.backgroundErrs = append(ts.backgroundErrs, errc)

	pubScanner := bufio.NewScanner(outrc) // TODO muxrpc comms?
	r.True(pubScanner.Scan(), "multiple lines of output from js - expected #1 to be alices pubkey/id")

	alice, err := ssb.ParseFeedRef(pubScanner.Text())
	r.NoError(err, "failed to get alice key from JS process")
	ts.t.Logf("JS alice: %s", alice.Ref())
	return alice
}

func (ts *testSession) wait() {
	closeErrc := make(chan error)

	go func() {
		tick := time.NewTicker(2 * time.Minute) // would be nice to get -test.timeout for this
		select {
		case <-ts.doneJS:

		case <-ts.doneGo:

		case <-tick.C:

		}

		ts.gobot.Shutdown()
		closeErrc <- ts.gobot.Close()
		close(closeErrc)
	}()

	for err := range mergeErrorChans(append(ts.backgroundErrs, closeErrc)...) {
		require.NoError(ts.t, err)
	}
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
