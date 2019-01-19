// sbotcli implements a simple tool to query commands on another sbot
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cryptix/go/logging"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	goon "github.com/shurcooL/go-goon"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/debug"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
	cli "gopkg.in/urfave/cli.v2"
)

var (
	sbotAppKey     []byte
	defaultKeyFile string

	longctx      context.Context
	shutdownFunc func()

	pkr    muxrpc.Packer
	client muxrpc.Endpoint

	log   logging.Interface
	check = logging.CheckFatal
)

func init() {
	var err error
	sbotAppKey, err = base64.StdEncoding.DecodeString("1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=")
	check(err)

	u, err := user.Current()
	check(err)

	defaultKeyFile = filepath.Join(u.HomeDir, ".ssb-go", "secret")
}

var Revision = "unset"

var streamFlags = []cli.Flag{
	&cli.IntFlag{Name: "limit", Value: -1},
	&cli.IntFlag{Name: "seq", Value: 0},
	&cli.BoolFlag{Name: "reverse"},
	&cli.BoolFlag{Name: "live"},
	&cli.BoolFlag{Name: "keys", Value: false},
	&cli.BoolFlag{Name: "values", Value: false},
}

func main() {
	logging.SetupLogging(nil)
	log = logging.Logger("cli")

	app := cli.App{
		Name:    os.Args[0],
		Usage:   "what can I say? sbot in Go",
		Version: "alpha2",
	}
	cli.VersionPrinter = func(c *cli.Context) {
		// go install -ldflags="-X main.Revision=$(git rev-parse HEAD)"
		fmt.Printf("%s ( rev: %s )\n", c.App.Version, Revision)
	}

	app.Flags = []cli.Flag{
		&cli.StringFlag{Name: "addr", Value: "localhost:8008", Usage: "tcp address of the sbot to connect to (or listen on)"},
		&cli.StringFlag{Name: "remoteKey", Value: "", Usage: "the remote pubkey you are connecting to (by default the local key)"},
		&cli.StringFlag{Name: "key,k", Value: defaultKeyFile},
		&cli.BoolFlag{Name: "verbose,vv", Usage: "print muxrpc packets"},
	}
	app.Before = initClient
	app.Commands = []*cli.Command{
		{
			Name:   "log",
			Action: logStreamCmd,
			Flags:  streamFlags,
		},
		{
			Name:   "hist",
			Action: historyStreamCmd,
			Flags:  append(streamFlags, &cli.StringFlag{Name: "id"}),
		},
		{
			Name:   "qry",
			Action: todo, //query,
		},
		{
			Name:   "call",
			Action: callCmd,
			Usage:  "make an dump* async call",
			UsageText: `SUPPORTS:
* whoami
* latestSequence
* getLatest
* get
* blobs.(has|want|rm|wants)
* gossip.(peers|add|connect)


see https://scuttlebot.io/apis/scuttlebot/ssb.html#createlogstream-source  for more

CAVEAT: only one argument...
`,
		},
		{
			Name:   "connect",
			Action: connectCmd,
			Usage:  "connect to a remote peer",
		},
		{
			Name: "private",
			Subcommands: []*cli.Command{
				{
					Name:   "read",
					Action: privateReadCmd,
					Flags:  streamFlags,
				},
			},
		},
		{
			Name:  "publish",
			Usage: "p",
			Subcommands: []*cli.Command{
				{
					Name:   "post",
					Action: publishPostCmd,
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "text", Value: "Hello, World!"},
						&cli.StringFlag{Name: "root", Value: "", Usage: "the ID of the first message of the thread"},
						// TODO: Slice of branches
						&cli.StringFlag{Name: "branch", Value: "", Usage: "the post ID that is beeing replied to"},
						&cli.StringSliceFlag{Name: "recps", Usage: "as a PM to these feeds"},
					},
				},
				{
					Name:   "about",
					Action: publishAboutCmd,
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "about", Usage: "who to assert"},
						&cli.StringFlag{Name: "name", Usage: "what name to give"},
						&cli.StringFlag{Name: "image", Usage: "image blob ref"},
					},
				},
				{
					Name:   "contact",
					Action: publishContactCmd,
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "contact", Usage: "who to (un)follow or block"},
						&cli.BoolFlag{Name: "following"},
						&cli.BoolFlag{Name: "blocking"},
						&cli.StringSliceFlag{Name: "recps", Usage: "as a PM to these feeds"},
					},
				},
				{
					Name:   "vote",
					Action: publishVoteCmd,
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "link", Usage: "the message ref to vote on"},
						&cli.IntFlag{Name: "value", Usage: "usually 1 (like) or 0 (unlike)"},
						&cli.StringFlag{Name: "expression", Usage: "Dig/Yup/Heart"},

						&cli.StringFlag{Name: "root", Value: "", Usage: "the ID of the first message of the thread"},
						// TODO: Slice of branches
						&cli.StringFlag{Name: "branch", Value: "", Usage: "the post ID that is beeing replied to"},

						&cli.StringSliceFlag{Name: "recps", Usage: "as a PM to these feeds"},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Log("runErr", err)
	}
	log.Log("pkrClose", pkr.Close())
}

func todo(ctx *cli.Context) error {
	return errors.Errorf("todo: %s", ctx.Command.Name)
}

func initClient(ctx *cli.Context) error {
	localKey, err := ssb.LoadKeyPair(ctx.String("key"))
	if err != nil {
		return err
	}

	c, err := secretstream.NewClient(localKey.Pair, sbotAppKey)
	if err != nil {
		return errors.Wrap(err, "error creating secretstream.Client")
	}
	var remotPubKey = localKey.Pair.Public
	if rk := ctx.String("remoteKey"); rk != "" {
		rk = strings.TrimSuffix(rk, ".ed25519")
		rk = strings.TrimPrefix(rk, "@")
		rpk, err := base64.StdEncoding.DecodeString(rk)
		if err != nil {
			return errors.Wrapf(err, "init: base64 decode of --remoteKey failed")
		}
		copy(remotPubKey[:], rpk)
	}

	plainAddr, err := net.ResolveTCPAddr("tcp", ctx.String("addr"))
	if err != nil {
		return errors.Wrapf(err, "init: base64 decode of --remoteKey failed")
	}

	conn, err := netwrap.Dial(plainAddr, c.ConnWrapper(remotPubKey))
	if err != nil {
		return errors.Wrap(err, "error dialing")
	}
	/* coming soon:
	conn, err := net.Dial("unix", "/home/cryptix/.ssb/socket")
	if err != nil {
		return errors.Wrap(err, "error dialing unix sock")
	}
	*/
	var rwc io.ReadWriteCloser = conn
	// logs every muxrpc packet
	if ctx.Bool("verbose") {
		rwc = debug.Wrap(log, rwc)
	}
	pkr = muxrpc.NewPacker(rwc)

	h := noopHandler{kitlog.With(log, "unit", "noop")}
	client = muxrpc.HandleWithRemote(pkr, &h, conn.RemoteAddr())

	longctx = context.Background()
	longctx, shutdownFunc = context.WithCancel(longctx)
	signalc := make(chan os.Signal)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalc
		fmt.Println("killed. shutting down")
		shutdownFunc()
		time.Sleep(1 * time.Second)
		check(pkr.Close())
		os.Exit(0)
	}()
	logging.SetCloseChan(signalc)
	go func() {
		err := client.(muxrpc.Server).Serve(longctx)
		check(err)
	}()
	log.Log("init", "done")
	return nil
}

type noopHandler struct {
	log logging.Interface
}

func (h noopHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	srv := edp.(muxrpc.Server)
	h.log.Log("event", "onConnect", "addr", srv.Remote())
}

func (h noopHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	h.log.Log("event", "onCall", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
}

func getStreamArgs(ctx *cli.Context) message.CreateHistArgs {
	return message.CreateHistArgs{
		Id:      ctx.String("id"),
		Limit:   ctx.Int64("limit"),
		Seq:     ctx.Int64("seq"),
		Live:    ctx.Bool("live"),
		Reverse: ctx.Bool("reverse"),
		Keys:    ctx.Bool("keys"),
		Values:  ctx.Bool("values"),
	}
}

func historyStreamCmd(ctx *cli.Context) error {
	var args = getStreamArgs(ctx)
	src, err := client.Source(longctx, map[string]interface{}{}, muxrpc.Method{"createHistoryStream"}, args)
	if err != nil {
		return errors.Wrap(err, "source stream call failed")
	}

	for {
		v, err := src.Next(longctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "creatHist(%s): failed to drain", args.Id)
		}
		goon.Dump(v)
	}
	return nil
}

func callCmd(ctx *cli.Context) error {
	cmd := ctx.Args().Get(0)
	if cmd == "" {
		return errors.New("call: cmd can't be empty")
	}
	args := ctx.Args().Slice()
	v := strings.Split(cmd, ".")
	var sendArgs []interface{}
	if len(args) > 0 {
		sendArgs = make([]interface{}, len(args))
		for i, v := range args {
			sendArgs[i] = v
		}
	}
	val, err := client.Async(longctx, map[string]interface{}{}, muxrpc.Method(v), sendArgs...) // TODO: args[1:]...
	if err != nil {
		return errors.Wrapf(err, "%s: call failed.", cmd)
	}
	log.Log("event", "call reply")
	goon.Dump(val)
	return nil
}

func connectCmd(ctx *cli.Context) error {
	to := ctx.Args().Get(0)
	if to == "" {
		return errors.New("connect: host argument can't be empty")
	}
	fields := strings.Split(to, ":")
	if n := len(fields); n != 3 {
		return errors.Errorf("connect: expecting host:port:pubkey - only got %d fields.", n)
	}
	val, err := client.Async(longctx, map[string]interface{}{}, muxrpc.Method{"ctrl", "connect"}, to)
	if err != nil {
		return errors.Wrapf(err, "connect: async call failed.")
	}
	log.Log("event", "connect reply")
	goon.Dump(val)
	return nil
}

func publishPostCmd(ctx *cli.Context) error {
	arg := map[string]interface{}{
		"text": ctx.String("text"),
		"type": "post",
	}
	if r := ctx.String("root"); r != "" {
		arg["root"] = r
		if b := ctx.String("branch"); b != "" {
			arg["branch"] = b
		} else {
			arg["branch"] = r
		}
	}
	type reply map[string]interface{}
	var v interface{}
	var err error
	if recps := ctx.StringSlice("recps"); len(recps) > 0 {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"private", "publish"}, arg, recps)
	} else {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"publish"}, arg)
	}
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}

	log.Log("event", "published", "type", "post")
	goon.Dump(v)
	return nil
}

func publishVoteCmd(ctx *cli.Context) error {
	mref, err := ssb.ParseMessageRef(ctx.String("link"))
	if err != nil {
		return errors.Wrapf(err, "publish/vote: invalid msg ref")
	}

	arg := map[string]interface{}{
		"vote": map[string]interface{}{
			"link":       mref.Ref(),
			"value":      ctx.Int("value"),
			"expression": ctx.String("expression"),
		},
		"type": "vote",
	}

	if r := ctx.String("root"); r != "" {
		arg["root"] = r
		if b := ctx.String("branch"); b != "" {
			arg["branch"] = b
		} else {
			arg["branch"] = r
		}
	}

	type reply map[string]interface{}
	var v interface{}
	if recps := ctx.StringSlice("recps"); len(recps) > 0 {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"private", "publish"}, arg, recps)
	} else {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"publish"}, arg)
	}
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}

	log.Log("event", "published", "type", "vote")
	goon.Dump(v)
	return nil
}

func publishAboutCmd(ctx *cli.Context) error {
	aboutRef, err := ssb.ParseFeedRef(ctx.String("about"))
	if err != nil {
		return errors.Wrapf(err, "publish/about: invalid feed ref")
	}
	if ctx.Bool("following") && ctx.Bool("blocking") {
		return errors.Errorf("publish/about: can't be both true")
	}
	arg := map[string]interface{}{
		"about": aboutRef.Ref(),
		"type":  "about",
	}
	if n := ctx.String("name"); n != "" {
		arg["name"] = n
	}
	if img := ctx.String("image"); img != "" {
		blobRef, err := ssb.ParseBlobRef(img)
		if err != nil {
			return errors.Wrapf(err, "publish/about: invalid blob ref")
		}
		arg["image"] = blobRef
	}
	type reply map[string]interface{}
	v, err := client.Async(longctx, reply{}, muxrpc.Method{"publish"}, arg)
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}
	log.Log("event", "published", "type", "about")
	goon.Dump(v)
	return nil
}

func publishContactCmd(ctx *cli.Context) error {
	cref, err := ssb.ParseFeedRef(ctx.String("contact"))
	if err != nil {
		return errors.Wrapf(err, "publish/contact: invalid feed ref")
	}
	if ctx.Bool("following") && ctx.Bool("blocking") {
		return errors.Errorf("publish/contact: can't be both true")
	}
	arg := map[string]interface{}{
		"contact":   cref.Ref(),
		"type":      "contact",
		"following": ctx.Bool("following"),
		"blocking":  ctx.Bool("blocking"),
	}
	type reply map[string]interface{}
	v, err := client.Async(longctx, reply{}, muxrpc.Method{"publish"}, arg)
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}
	log.Log("event", "published", "type", "contact")
	goon.Dump(v)
	return nil
}

func logStreamCmd(ctx *cli.Context) error {
	src, err := client.Source(longctx, map[string]interface{}{}, muxrpc.Method{"createLogStream"})
	if err != nil {
		return errors.Wrap(err, "source stream call failed")
	}
	var msgs []interface{}
	for {
		v, err := src.Next(longctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "createLogStream: failed to drain")
		}
		msgs = append(msgs, v)
	}
	return json.NewEncoder(os.Stdout).Encode(msgs)
}

func privateReadCmd(ctx *cli.Context) error {
	var args = getStreamArgs(ctx)
	src, err := client.Source(longctx, map[string]interface{}{}, muxrpc.Method{"private", "read"}, args)
	if err != nil {
		return errors.Wrap(err, "source stream call failed")
	}
	for {
		v, err := src.Next(longctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "createLogStream: failed to drain")
		}
		goon.Dump(v)
	}
	return nil
}

/*

func query(ctx *cli.Context) error {
	reply := make(chan map[string]interface{})
	go func() {
		for r := range reply {
			goon.Dump(r)
		}
	}()
	if err := client.Source("query.read", reply, ctx.Args().Get(0)); err != nil {
		return errors.Wrap(err, "source stream call failed")
	}
	return client.Close()
}







*/
