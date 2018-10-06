package tests

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os/exec"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
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

func initInterop(t *testing.T, jsbefore, jsafter string) *sbot.Sbot {
	r := require.New(t)
	ctx := context.Background()

	dir, err := ioutil.TempDir("", t.Name())
	r.NoError(err, "failed to create testdir for repo")
	info, _ := logtest.KitLogger("go", t)
	sbot, err := sbot.New(
		sbot.WithInfo(info),
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
	cmd.Stderr = logtest.Logger("js", t)
	cmd.Stdout = b
	cmd.Env = []string{
		"TEST_NAME=" + t.Name(),
		"TEST_BOB=" + sbot.KeyPair.Id.Ref(),
		"TEST_GOADDR=" + netwrap.GetAddr(sbot.Node.GetListenAddr(), "tcp").String(),
		"TEST_BEFORE=" + writeFile(t, jsbefore),
		"TEST_AFTER=" + writeFile(t, jsafter),
	}

	r.NoError(cmd.Run(), "failed to init test js-sbot")
	t.Logf("JSbot: %s", b.String())

	// time.Sleep(3 * time.Second)
	// r.NoError(sbot.Close(), "failed to close go-sbot")
	return sbot
}

func TestInteropFeedsJS2GO(t *testing.T) {
	initInterop(t, `
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 50
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test",text:"foo",i:1}))
	}
	series(msgs, function(err, results) {
		t.error(err, "series of publish")
		t.equal(n, results.length, "message count")
		run() // triggers connect and after block
	})
`, `
pull(
	sbot.createUserStream({id:alice.id}),
	pull.collect(function(err, vals){
		t.equal(n, vals.length)
		t.end(err)
	})
)
`)
}

func TestInteropBlobs(t *testing.T) {
	r := require.New(t)

	testRef, err := ssb.ParseBlobRef("&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256") // foobar
	r.NoError(err)

	s := initInterop(t,
		`pull(
			pull.values([Buffer.from("foobar")]),
			sbot.blobs.add(function(err, id) {
				t.error(err)
				t.equal(id, '&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256')
				run()
			})
		)`,
		`sbot.blobs.has(
			"&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256",
			function(err, has) {
				t.true(has, "should have blob")
				t.end(err)
			})`)

	err = s.WantManager.Want(testRef)
	r.NoError(err, ".Want() should not error")

	time.Sleep(5 * time.Second)

	br, err := s.BlobStore.Get(testRef)
	r.NoError(err, "should have blob")

	foobar, err := ioutil.ReadAll(br)
	r.NoError(err, "couldnt read blob")
	r.Equal("foobar", string(foobar))
}
