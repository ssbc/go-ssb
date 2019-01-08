// sbotcli implements a simple tool to query commands on another sbot
package main

import (
	"context"
	"encoding/base64"
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
			Action: todo, //logStreamCmd,
		},
		{
			Name:   "hist",
			Action: historyStreamCmd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "id"},
				&cli.IntFlag{Name: "limit", Value: -1},
				&cli.IntFlag{Name: "seq", Value: 0},
				&cli.BoolFlag{Name: "reverse"},
				&cli.BoolFlag{Name: "live"},
				&cli.BoolFlag{Name: "keys", Value: true},
				&cli.BoolFlag{Name: "values", Value: true},
			},
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
					Name:   "publish",
					Usage:  "p",
					Action: todo, //privatePublishCmd,
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "type", Value: "post"},
						&cli.StringFlag{Name: "text", Value: "Hello, World!"},
						&cli.StringFlag{Name: "root", Usage: "the ID of the first message of the thread"},
						&cli.StringFlag{Name: "branch", Usage: "the post ID that is beeing replied to"},
						&cli.StringFlag{Name: "channel"},
						&cli.StringSliceFlag{Name: "recps", Usage: "posting to these IDs privatly"},
					},
				},
				{
					Name:   "unbox",
					Usage:  "u",
					Action: todo, //privateUnboxCmd,
				},
			},
		},
		{
			Name:   "publish",
			Usage:  "p",
			Action: todo, //publishCmd,
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "type", Value: "post"},
				&cli.StringFlag{Name: "text", Value: "Hello, World!"},
				&cli.StringFlag{Name: "root", Value: "", Usage: "the ID of the first message of the thread"},
				&cli.StringFlag{Name: "branch", Value: "", Usage: "the post ID that is beeing replied to"},
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

func historyStreamCmd(ctx *cli.Context) error {
	var args = message.CreateHistArgs{
		Id:      ctx.String("id"),
		Limit:   ctx.Int64("limit"),
		Seq:     ctx.Int64("seq"),
		Live:    ctx.Bool("live"),
		Reverse: ctx.Bool("reverse"),
		Keys:    ctx.Bool("keys"),
		Values:  ctx.Bool("values"),
	}
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
	val, err := client.Async(longctx, map[string]interface{}{}, muxrpc.Method{"gossip", "connect"}, to)
	if err != nil {
		return errors.Wrapf(err, "connect: async call failed.")
	}
	log.Log("event", "connect reply")
	goon.Dump(val)
	return nil
}

/*

func logStreamCmd(ctx *cli.Context) error {
	reply := make(chan map[string]interface{})
	go func() {
		for r := range reply {
			goon.Dump(r)
		}
	}()
	if err := client.Source("createLogStream", reply); err != nil {
		return errors.Wrap(err, "source stream call failed")
	}
	return client.Close()
}


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


func privatePublishCmd(ctx *cli.Context) error {
	content := map[string]interface{}{
		"text": ctx.String("text"),
		"type": ctx.String("type"),
	}
	if c := ctx.String("channel"); c != "" {
		content["channel"] = c
	}
	if r := ctx.String("root"); r != "" {
		content["root"] = r
		if b := ctx.String("branch"); b != "" {
			content["branch"] = b
		} else {
			content["branch"] = r
		}
	}
	recps := ctx.StringSlice("recps")
	if len(recps) == 0 {
		return errors.Errorf("private.publish: 0 recps.. that would be quite the lonely message..")
	}
	var reply map[string]interface{}
	err := client.Call("private.publish", &reply, content, recps)
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}
	log.Log("event", "private published")
	goon.Dump(reply)
	return client.Close()
}


func privateUnboxCmd(ctx *cli.Context) error {
	id := ctx.Args().Get(0)
	if id == "" {
		return errors.New("get: id can't be empty")
	}
	var getReply map[string]interface{}
	if err := client.Call("get", id, &getReply); err != nil {
		return errors.Wrapf(err, "get call failed.")
	}
	log.Log("event", "get reply")
	goon.Dump(getReply)
	var reply map[string]interface{}
	if err := client.Call("private.unbox", getReply["content"], &reply); err != nil {
		return errors.Wrapf(err, "get call failed.")
	}
	log.Log("event", "unboxed")
	goon.Dump(reply)
	return client.Close()
}

func publishCmd(ctx *cli.Context) error {
	arg := map[string]interface{}{
		"text": ctx.String("text"),
		"type": ctx.String("type"),
	}
	if r := ctx.String("root"); r != "" {
		arg["root"] = r
		if b := ctx.String("branch"); b != "" {
			arg["branch"] = b
		} else {
			arg["branch"] = r
		}
	}
	var reply map[string]interface{}
	err := client.Call("publish", arg, &reply)
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}
	log.Log("event", "published")
	goon.Dump(reply)
	return client.Close()
}


*/
