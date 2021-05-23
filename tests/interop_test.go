// SPDX-License-Identifier: MIT

// Package tests contains test scenarios and helpers to run interoparability tests against the javascript implementation.
package tests

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	mrand "math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2/debug"
	"go.cryptoscope.co/netwrap"
	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/internal/testutils"
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

	info log.Logger

	repo string

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
	repo := filepath.Join("testrun", t.Name())
	os.RemoveAll(repo)

	return &testSession{
		info:    testutils.NewRelativeTimeLogger(nil),
		repo:    repo,
		t:       t,
		keySHS:  appKey,
		keyHMAC: hmacKey,
	}
}

// returns the created go-sbot, the pubkey of the jsbot, a wait and a cleanup function
func (ts *testSession) startGoBot(sbotOpts ...sbot.Option) {
	r := require.New(ts.t)
	ctx := context.Background()

	// prepend defaults
	sbotOpts = append([]sbot.Option{
		sbot.WithInfo(ts.info),
		sbot.WithListenAddr("localhost:0"),
		sbot.WithRepoPath(ts.repo),
		sbot.WithContext(ctx),
	}, sbotOpts...)

	if ts.keySHS != nil {
		sbotOpts = append(sbotOpts, sbot.WithAppKey(ts.keySHS))
	}
	if ts.keyHMAC != nil {
		sbotOpts = append(sbotOpts, sbot.WithHMACSigning(ts.keyHMAC))
	}

	sbotOpts = append(sbotOpts,
		sbot.WithPostSecureConnWrapper(func(conn net.Conn) (net.Conn, error) {
			return debug.WrapDump(filepath.Join("testrun", ts.t.Name(), "muxdump"), conn)
		}),
	)

	sbot, err := sbot.New(sbotOpts...)
	r.NoError(err, "failed to init test go-sbot")
	ts.t.Logf("go-sbot: %s", sbot.KeyPair.Id.Ref())

	var done = make(chan struct{})
	var errc = make(chan error, 1)
	go func() {
		err := sbot.Network.Serve(ctx)
		if err != nil {
			errc <- fmt.Errorf("node serve exited: %w", err)
		}
		// ts.t.Log("go-sbot: serve exited", err)
		close(done)
		close(errc)
	}()
	ts.doneGo = done
	ts.backgroundErrs = append(ts.backgroundErrs, errc)
	ts.gobot = sbot

	// TODO: make muxrpc client and connect to whoami for _ready_ ?
	return
}

var jsBotCnt = 0

func (ts *testSession) startJSBot(jsbefore, jsafter string) refs.FeedRef {
	return ts.startJSBotWithName("", jsbefore, jsafter)
}

// returns the jsbots pubkey
func (ts *testSession) startJSBotWithName(name, jsbefore, jsafter string) refs.FeedRef {
	ts.t.Log("starting client", name)
	r := require.New(ts.t)
	cmd := exec.Command("node", "./sbot_client.js")
	cmd.Stderr = os.Stderr

	outrc, err := cmd.StdoutPipe()
	r.NoError(err)

	if name == "" {
		name = fmt.Sprint(ts.t.Name(), jsBotCnt)
	}
	jsBotCnt++
	env := []string{
		"TEST_NAME=" + name,
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
			errc <- fmt.Errorf("cmd wait failed: %w", err)
		}
		close(done)
		fmt.Fprintf(os.Stderr, "\nJS Sbot process returned\n")
		close(errc)
	}()
	ts.doneJS = done // TODO: multiple
	ts.backgroundErrs = append(ts.backgroundErrs, errc)

	pubScanner := bufio.NewScanner(outrc) // TODO muxrpc comms?
	r.True(pubScanner.Scan(), "multiple lines of output from js - expected #1 to be %s pubkey/id", name)

	jsBotRef, err := refs.ParseFeedRef(pubScanner.Text())
	r.NoError(err, "failed to get %s key from JS process")
	ts.t.Logf("JS %s:%d %s", name, jsBotCnt, jsBotRef.Ref())
	return jsBotRef
}

func (ts *testSession) startJSBotAsServer(name, jsbefore, jsafter string) (refs.FeedRef, int) {
	ts.t.Log("starting srv", name)
	r := require.New(ts.t)
	cmd := exec.Command("node", "./sbot_serv.js")
	cmd.Stderr = os.Stderr

	outrc, err := cmd.StdoutPipe()
	r.NoError(err)

	if name == "" {
		name = fmt.Sprint(ts.t.Name(), jsBotCnt)
	}
	jsBotCnt++

	var port = 1024 + mrand.Intn(23000)

	env := []string{
		"TEST_NAME=" + name,
		"TEST_BOB=" + ts.gobot.KeyPair.Id.Ref(),
		fmt.Sprintf("TEST_PORT=%d", port),
		"TEST_BEFORE=" + writeFile(ts.t, jsbefore),
		"TEST_AFTER=" + writeFile(ts.t, jsafter),
	}
	if ts.keySHS != nil {
		env = append(env, "TEST_APPKEY="+base64.StdEncoding.EncodeToString(ts.keySHS))
	}
	if ts.keyHMAC != nil {
		ts.t.Fatal("fix HMAC setup")
		env = append(env, "TEST_HMACKEY="+base64.StdEncoding.EncodeToString(ts.keyHMAC))
	}
	cmd.Env = env

	r.NoError(cmd.Start(), "failed to init test js-sbot")

	var done = make(chan struct{})
	var errc = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			errc <- fmt.Errorf("cmd wait failed: %w", err)
		}
		close(done)
		fmt.Fprintf(os.Stderr, "\nJS Sbot process returned\n")
		close(errc)
	}()
	ts.doneJS = done // TODO: multiple
	ts.backgroundErrs = append(ts.backgroundErrs, errc)

	pubScanner := bufio.NewScanner(outrc) // TODO muxrpc comms?
	r.True(pubScanner.Scan(), "multiple lines of output from js - expected #1 to be %s pubkey/id", name)

	srvRef, err := refs.ParseFeedRef(pubScanner.Text())
	r.NoError(err, "failed to get srvRef key from JS process")
	ts.t.Logf("JS %s: %s port: %d", name, srvRef.Ref(), port)
	return srvRef, port
}

func (ts *testSession) wait() {
	closeErrc := make(chan error)

	go func() {
		tick := time.NewTicker(30 * time.Second) // would be nice to get -test.timeout for this
		select {
		case <-ts.doneJS:

		case <-ts.doneGo:

		case <-tick.C:
			ts.t.Log("timeout")
		}

		require.NoError(ts.t, ts.gobot.FSCK(sbot.FSCKWithMode(sbot.FSCKModeSequences)))
		ts.gobot.Shutdown()
		closeErrc <- ts.gobot.Close()
		close(closeErrc)
	}()

	for err := range testutils.MergeErrorChans(append(ts.backgroundErrs, closeErrc)...) {
		require.NoError(ts.t, err)
	}
}
