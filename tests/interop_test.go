package tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/sbot/message"
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

func initInterop(t *testing.T, jsbefore, jsafter string) (*sbot.Sbot, *ssb.FeedRef) {
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

	out := bufio.NewScanner(b)
	i := 0
	var last string
	for out.Scan() {
		s := out.Text()
		// t.Logf("out(%d): %q", i, s)
		i++
		last = s
	}

	r.Equal(i, 1, "multiple lines of output from js - expected #1 to be alices pubkey/id")
	alice, err := ssb.ParseFeedRef(last)
	r.NoError(err, "failed to get alice key from JS process")
	t.Logf("JS alice: %s", alice.Ref())

	return sbot, alice
}

func TestInteropFeedsJS2GO(t *testing.T) {
	r := require.New(t)
	s, alice := initInterop(t, `
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 50
	let msgs = []
	for (var i = 50; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", i:i}))
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
	aliceLog, err := s.UserFeeds.Get(librarian.Addr(alice.ID))
	r.NoError(err)
	seq, err := aliceLog.Seq().Value()
	r.NoError(err)
	r.Equal(seq, margaret.BaseSeq(49))

	for i := 0; i < 50; i++ {
		// only one feed in log - directly the rootlog sequences
		seqMsg, err := aliceLog.Get(margaret.BaseSeq(i))
		r.NoError(err)
		r.Equal(seqMsg, margaret.BaseSeq(i))

		msg, err := s.RootLog.Get(seqMsg.(margaret.BaseSeq))
		r.NoError(err)
		storedMsg, ok := msg.(message.StoredMessage)
		r.True(ok, "wrong type of message: %T", msg)
		r.Equal(storedMsg.Sequence, margaret.BaseSeq(i+1))

		type testWrap struct {
			Author  ssb.FeedRef
			Content struct {
				Type, Text string
				I          int
			}
		}
		var m testWrap
		err = json.Unmarshal(storedMsg.Raw, &m)
		r.NoError(err)
		r.Equal(alice.ID, m.Author.ID, "wrong author")
		r.Equal(m.Content.Type, "test")
		r.Equal(m.Content.Text, "foo")
		r.Equal(m.Content.I, 50-i, "wrong I on msg: %d", i)
	}
}

func TestInteropBlobs(t *testing.T) {
	r := require.New(t)

	testRef, err := ssb.ParseBlobRef("&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256") // foobar
	r.NoError(err)

	s, _ := initInterop(t,
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
