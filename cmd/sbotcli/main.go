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
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/debug"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
	cli "gopkg.in/urfave/cli.v2"
)

var (
	longctx      context.Context
	shutdownFunc func()

	pkr    muxrpc.Packer
	client muxrpc.Endpoint

	log   logging.Interface
	check = logging.CheckFatal

	keyFileFlag = cli.StringFlag{Name: "key,k", Value: "unset"}
)

func init() {
	u, err := user.Current()
	check(err)

	keyFileFlag.Value = filepath.Join(u.HomeDir, ".ssb-go", "secret")
}

var Revision = "unset"

var app = cli.App{
	Name:    os.Args[0],
	Usage:   "client for controlling Cryptoscope's SSB server",
	Version: "alpha3",

	Flags: []cli.Flag{
		&cli.StringFlag{Name: "shscap", Value: "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=", Usage: "shs key"},
		&cli.StringFlag{Name: "addr", Value: "localhost:8008", Usage: "tcp address of the sbot to connect to (or listen on)"},
		&cli.StringFlag{Name: "remoteKey", Value: "", Usage: "the remote pubkey you are connecting to (by default the local key)"},
		&keyFileFlag,
		&cli.BoolFlag{Name: "verbose,vv", Usage: "print muxrpc packets"},
	},

	Before: initClient,
	Commands: []*cli.Command{
		logStreamCmd,
		typeStreamCmd,
		historyStreamCmd,
		replicateUptoCmd,
		callCmd,
		connectCmd,
		queryCmd,
		privateCmd,
		publishCmd,
	},
}

func main() {
	logging.SetupLogging(nil)
	log = logging.Logger("cli")

	cli.VersionPrinter = func(c *cli.Context) {
		// go install -ldflags="-X main.Revision=$(git rev-parse HEAD)"
		fmt.Printf("%s ( rev: %s )\n", c.App.Version, Revision)
	}

	if err := app.Run(os.Args); err != nil {
		log.Log("runErr", err)
	}
	if pkr != nil {
		log.Log("pkrClose", pkr.Close())
	}
}

func todo(ctx *cli.Context) error {
	return errors.Errorf("todo: %s", ctx.Command.Name)
}

func initClient(ctx *cli.Context) error {
	localKey, err := ssb.LoadKeyPair(ctx.String("key"))
	if err != nil {
		return err
	}

	shscap, err := base64.StdEncoding.DecodeString(ctx.String("shscap"))
	if err != nil {
		return errors.Wrap(err, "shs capability decode failed")
	}

	c, err := secretstream.NewClient(localKey.Pair, shscap)
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
	h.log.Log("event", "onCall", "args", fmt.Sprintf("%v", req.Args), "method", req.Method, "type", req.Type)
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

var callCmd = &cli.Command{
	Name:  "call",
	Usage: "make an dump* async call",
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
	Action: func(ctx *cli.Context) error {
		cmd := ctx.Args().Get(0)
		if cmd == "" {
			return errors.New("call: cmd can't be empty")
		}
		args := ctx.Args().Slice()
		v := strings.Split(cmd, ".")
		var sendArgs []interface{}
		if len(args) > 1 {
			sendArgs = make([]interface{}, len(args)-1)
			for i, v := range args[1:] {
				sendArgs[i] = v
			}
		}
		var reply interface{}
		val, err := client.Async(longctx, reply, muxrpc.Method(v), sendArgs...) // TODO: args[1:]...
		if err != nil {
			return errors.Wrapf(err, "%s: call failed.", cmd)
		}
		log.Log("event", "call reply")
		goon.Dump(val)
		return nil
	},
}

var connectCmd = &cli.Command{
	Name:  "connect",
	Usage: "connect to a remote peer",
	Action: func(ctx *cli.Context) error {
		to := ctx.Args().Get(0)
		if to == "" {
			return errors.New("connect: multiserv addr argument can't be empty")
		}
		var val interface{}
		val, err := client.Async(longctx, val, muxrpc.Method{"ctrl", "connect"}, to)
		if err != nil {
			return errors.Wrapf(err, "connect: async call failed.")
		}
		log.Log("event", "connect reply")
		goon.Dump(val)
		return nil
	},
}

var queryCmd = &cli.Command{
	Name:   "qry",
	Action: todo, //query,
}

var privateCmd = &cli.Command{
	Name: "private",
	Subcommands: []*cli.Command{
		privateReadCmd,
	},
}
