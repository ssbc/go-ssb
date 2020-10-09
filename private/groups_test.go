package private_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	kitlog "github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/sbot"
)

func TestGroupsInit(t *testing.T) {
	r := require.New(t)
	// a := assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)

	alice, err := ssb.NewKeyPair(bytes.NewReader(bytes.Repeat([]byte("alice"), 8)))
	r.NoError(err)

	srvLog := kitlog.NewNopLogger()
	if testing.Verbose() {
		srvLog = kitlog.NewLogfmtLogger(os.Stderr)
	}

	// mlogPriv := multilogs.NewPrivateRead(kitlog.With(srvLog, "module", "privLogs"), alice)

	srv, err := sbot.New(
		sbot.WithKeyPair(alice),
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"),
		sbot.LateOption(sbot.WithUNIXSocket()),
		sbot.LateOption(sbot.MountSimpleIndex("get", indexes.OpenGet)), // todo muxrpc plugin is hardcoded
		// sbot.LateOption(sbot.MountMultiLog("privLogs", mlogPriv.OpenRoaring)),
	)
	r.NoError(err)

	_, err = srv.PublishLog.Publish(map[string]interface{}{"type": "test", "text": "hello, world!"})
	r.NoError(err)

	msgRef, err := srv.Groups.Init("hello, my group")
	r.NoError(err)
	r.NotNil(msgRef)

	t.Log(msgRef.ShortRef())

	msg, err := srv.Get(*msgRef)
	r.NoError(err)

	content := msg.ContentBytes()

	suffix := []byte(".box2\"")
	r.True(bytes.HasSuffix(content, suffix), "%q", content)

	n := base64.StdEncoding.DecodedLen(len(content))
	ctxt := make([]byte, n)
	decn, err := base64.StdEncoding.Decode(ctxt, bytes.TrimSuffix(content, suffix)[1:])
	r.NoError(err)
	ctxt = ctxt[:decn]

	clear, err := srv.Groups.DecryptBox2(ctxt, srv.KeyPair.Id, msg.Previous())
	r.NoError(err)
	t.Log(string(clear))

	srv.Shutdown()
	r.NoError(srv.Close())
}
