// SPDX-License-Identifier: MIT

// sbotcli implements a simple tool to query commands on another sbot
package main

import (
	"bufio"
	"bytes"
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

	"github.com/pkg/errors"
	goon "github.com/shurcooL/go-goon"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	kitlog "go.mindeco.de/log"
	"go.mindeco.de/log/level"
	"go.mindeco.de/log/term"
	"golang.org/x/crypto/ed25519"
	cli "gopkg.in/urfave/cli.v2"

	"go.cryptoscope.co/ssb"
	ssbClient "go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/plugins/legacyinvites"
	refs "go.mindeco.de/ssb-refs"
)

// Version and Build are set by ldflags
var (
	Version = "snapshot"
	Build   = ""
)

var (
	longctx      context.Context
	shutdownFunc func()

	log kitlog.Logger

	keyFileFlag  = cli.StringFlag{Name: "key,k", Value: "unset"}
	unixSockFlag = cli.StringFlag{Name: "unixsock", Usage: "if set, unix socket is used instead of tcp"}
)

func init() {
	u, err := user.Current()
	check(err)

	keyFileFlag.Value = filepath.Join(u.HomeDir, ".ssb-go", "secret")
	unixSockFlag.Value = filepath.Join(u.HomeDir, ".ssb-go", "socket")

	log = term.NewColorLogger(os.Stderr, kitlog.NewLogfmtLogger, colorFn)
}

var app = cli.App{
	Name:    os.Args[0],
	Usage:   "client for controlling Cryptoscope's SSB server",
	Version: "alpha4",

	Flags: []cli.Flag{
		&cli.StringFlag{Name: "shscap", Value: "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=", Usage: "shs key"},
		&cli.StringFlag{Name: "addr", Value: "localhost:8008", Usage: "tcp address of the sbot to connect to (or listen on)"},
		&cli.StringFlag{Name: "remoteKey", Value: "", Usage: "the remote pubkey you are connecting to (by default the local key)"},
		&keyFileFlag,
		&unixSockFlag,
		&cli.BoolFlag{Name: "verbose,vv", Usage: "print muxrpc packets"},

		&cli.StringFlag{Name: "timeout", Value: "45s", Usage: "pass a duration (like 3s or 5m) after which it times out, empty string to disable"},
	},

	Before: initClient,
	Commands: []*cli.Command{
		aliasCmd,
		blobsCmd,
		blockCmd,
		friendsCmd,
		getCmd,
		inviteCmds,
		logStreamCmd,
		sortedStreamCmd,
		typeStreamCmd,
		historyStreamCmd,
		partialStreamCmd,
		replicateUptoCmd,
		repliesStreamCmd,
		callCmd,
		sourceCmd,
		connectCmd,
		publishCmd,
		groupsCmd,
		// tryEBTCmd,
	},
}

// Color by error type
func colorFn(keyvals ...interface{}) term.FgBgColor {
	for i := 1; i < len(keyvals); i += 2 {
		if _, ok := keyvals[i].(error); ok {
			return term.FgBgColor{Fg: term.Red}
		}
	}
	return term.FgBgColor{}
}

func check(err error) {
	if err != nil {
		level.Error(log).Log("err", err)
		os.Exit(1)
	}
}

func main() {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s (rev: %s, built: %s)\n", c.App.Version, Version, Build)
	}

	if err := app.Run(os.Args); err != nil {
		level.Error(log).Log("run-failure", err)
	}
}

func todo(ctx *cli.Context) error {
	return fmt.Errorf("todo: %s", ctx.Command.Name)
}

func initClient(ctx *cli.Context) error {
	dstr := ctx.String("timeout")
	if dstr != "" {
		d, err := time.ParseDuration(dstr)
		if err != nil {
			return err
		}
		longctx, shutdownFunc = context.WithTimeout(context.Background(), d)
	} else {
		longctx, shutdownFunc = context.WithCancel(context.Background())
	}

	signalc := make(chan os.Signal)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-signalc
		level.Warn(log).Log("event", "shutting down", "sig", s)
		shutdownFunc()
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	return nil
}

func newClient(ctx *cli.Context) (*ssbClient.Client, error) {
	sockPath := ctx.String("unixsock")
	if sockPath != "" {
		client, err := ssbClient.NewUnix(sockPath, ssbClient.WithContext(longctx))
		if err != nil {
			return nil, errors.Wrap(err, "unix-path based client init failed")
		}
		return client, nil
	}

	// Assume TCP connection
	localKey, err := ssb.LoadKeyPair(ctx.String("key"))
	if err != nil {
		return nil, err
	}

	var remotePubKey = make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(remotePubKey, localKey.Pair.Public)
	if rk := ctx.String("remoteKey"); rk != "" {
		rk = strings.TrimSuffix(rk, ".ed25519")
		rk = strings.TrimPrefix(rk, "@")
		rpk, err := base64.StdEncoding.DecodeString(rk)
		if err != nil {
			return nil, errors.Wrapf(err, "init: base64 decode of --remoteKey failed")
		}
		copy(remotePubKey, rpk)
	}

	plainAddr, err := net.ResolveTCPAddr("tcp", ctx.String("addr"))
	if err != nil {
		return nil, errors.Wrapf(err, "int: failed to resolve TCP address")
	}

	shsAddr := netwrap.WrapAddr(plainAddr, secretstream.Addr{PubKey: remotePubKey})
	client, err := ssbClient.NewTCP(localKey, shsAddr,
		ssbClient.WithSHSAppKey(ctx.String("shscap")),
		ssbClient.WithContext(longctx))
	if err != nil {
		return nil, errors.Wrapf(err, "init: failed to connect to %s", shsAddr.String())
	}
	log.Log("init", "done")
	return client, nil
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

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var reply interface{}
		err = client.Async(longctx, &reply, muxrpc.TypeJSON, muxrpc.Method(v), sendArgs...) // TODO: args[1:]...
		if err != nil {
			return errors.Wrapf(err, "%s: call failed.", cmd)
		}
		level.Debug(log).Log("event", "call reply")
		jsonReply, err := json.MarshalIndent(reply, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "%s: indent failed.", cmd)
		}
		_, err = io.Copy(os.Stdout, bytes.NewReader(jsonReply))
		return errors.Wrapf(err, "%s: result copy failed.", cmd)
	},
}

var sourceCmd = &cli.Command{
	Name:  "source",
	Usage: "make an simple source call",

	Flags: []cli.Flag{
		&cli.StringFlag{Name: "id", Value: ""},
		// TODO: Slice of branches
		&cli.IntFlag{Name: "limit", Value: -1},
	},

	Action: func(ctx *cli.Context) error {
		cmd := ctx.Args().Get(0)
		if cmd == "" {
			return errors.New("call: cmd can't be empty")
		}

		v := strings.Split(cmd, ".")

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var args = struct {
			ID    string `json:"id"`
			Limit int    `json:"limit"`
		}{
			ID:    ctx.String("id"),
			Limit: ctx.Int("limit"),
		}

		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method(v), args)
		if err != nil {
			return errors.Wrapf(err, "%s: call failed.", cmd)
		}
		level.Debug(log).Log("event", "call reply")

		err = jsonDrain(os.Stdout, src)
		return errors.Wrapf(err, "%s: result copy failed.", cmd)
	},
}

var getCmd = &cli.Command{
	Name:  "get",
	Usage: "get a single message from the database by key (%...)",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "private"},
		&cli.StringFlag{Name: "format", Value: "json"},
	},
	Action: func(ctx *cli.Context) error {
		key, err := refs.ParseMessageRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to validate message ref: %w", err)
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		arg := struct {
			ID      *refs.MessageRef `json:"id"`
			Private bool             `json:"private"`
		}{key, ctx.Bool("private")}

		var val interface{}
		err = client.Async(longctx, &val, muxrpc.TypeJSON, muxrpc.Method{"get"}, arg)
		if err != nil {
			return err
		}
		format := strings.ToLower(ctx.String("format"))
		log.Log("event", "get reply", "format", format)
		switch format {
		case "json":
			indented, err := json.MarshalIndent(val, "", "  ")
			if err != nil {
				return err
			}
			os.Stdout.Write(indented)

		default:
			fmt.Printf("%+v\n", val)
		}
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

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var val string
		err = client.Async(longctx, &val, muxrpc.TypeString, muxrpc.Method{"ctrl", "connect"}, to)
		if err != nil {
			level.Warn(log).Log("event", "connect command failed", "err", err)

			// js fallback (our mux doesnt support authed namespaces)
			err = client.Async(longctx, &val, muxrpc.TypeString, muxrpc.Method{"gossip", "connect"}, to)
			if err != nil {
				return errors.Wrapf(err, "connect: async call failed.")
			}
		}
		log.Log("event", "connect reply")
		goon.Dump(val)
		return nil
	},
}

var blockCmd = &cli.Command{
	Name: "block",
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var blocked = make(map[*refs.FeedRef]bool)

		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fr, err := refs.ParseFeedRef(sc.Text())
			if err != nil {
				return err
			}
			blocked[fr] = true
		}
		log.Log("blocking", len(blocked))

		var val interface{}
		err = client.Async(longctx, val, muxrpc.TypeJSON, muxrpc.Method{"ctrl", "block"}, blocked)
		if err != nil {
			return err
		}
		log.Log("event", "block reply")
		goon.Dump(val)
		return nil
	},
}

var queryCmd = &cli.Command{
	Name:   "qry",
	Action: todo, //query,
}

/*
var getCmd = &cli.Command{
	Name:  "create",
	Usage: "create a new empty group",
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		getRef, err := refs.PraseMessageRef(ctx.Args().First())
		if err != nil {
			return err
		}

		msg, err = client.Get(getRef)
		if err != nil {
			return err
		}
		log.Log("event", "get response")
		goon.Dump(msg)
		return nil
	},
}
*/

var groupsCmd = &cli.Command{
	Name:  "groups",
	Usage: "group managment (create, invite, publishTo, etc.)",
	Subcommands: []*cli.Command{
		groupsCreateCmd,
		groupsInviteCmd,
		groupsPublishToCmd,
		groupsJoinCmd,
		/* TODO:
		groupsListCmd,
		groupsMembersCmd,
		*/
	},
}

var groupsCreateCmd = &cli.Command{
	Name:  "create",
	Usage: "create a new empty group",
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		name := ctx.Args().First()
		if name == "" {
			return fmt.Errorf("group name can't be empty")
		}

		var val interface{}
		err = client.Async(longctx, &val, muxrpc.TypeJSON, muxrpc.Method{"groups", "create"}, struct {
			Name string `json:"name"`
		}{name})
		if err != nil {
			return err
		}
		log.Log("event", "group created")
		goon.Dump(val)
		return nil
	},
}

var groupsInviteCmd = &cli.Command{
	Name:  "invite",
	Usage: "add people to a group",
	Action: func(ctx *cli.Context) error {
		args := ctx.Args()
		groupID, err := refs.ParseMessageRef(args.First())
		if err != nil {
			return fmt.Errorf("groupID needs to be a valid message ref: %w", err)
		}

		if groupID.Algo != refs.RefAlgoCloakedGroup {
			return fmt.Errorf("groupID needs to be a cloaked message ref, not %s", groupID.Algo)
		}

		member, err := refs.ParseFeedRef(args.Get(1))
		if err != nil {
			return err
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var reply interface{}
		err = client.Async(longctx, &reply, muxrpc.TypeJSON, muxrpc.Method{"groups", "invite"}, groupID.Ref(), member.Ref())
		if err != nil {
			return errors.Wrapf(err, "invite call failed")
		}
		log.Log("event", "member added", "group", groupID.Ref(), "member", member.Ref())
		goon.Dump(reply)
		return nil
	},
}

var groupsPublishToCmd = &cli.Command{
	Name:  "publishTo",
	Usage: "publish a handcrafted JSON blob to a group",
	Action: func(ctx *cli.Context) error {
		var content interface{}
		err := json.NewDecoder(os.Stdin).Decode(&content)
		if err != nil {
			return errors.Wrapf(err, "publish/raw: invalid json input from stdin")
		}

		groupID, err := refs.ParseMessageRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("groupID needs to be a valid message ref: %w", err)
		}

		if groupID.Algo != refs.RefAlgoCloakedGroup {
			return fmt.Errorf("groupID needs to be a cloaked message ref, not %s", groupID.Algo)
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var reply interface{}
		err = client.Async(longctx, &reply, muxrpc.TypeJSON, muxrpc.Method{"groups", "publishTo"}, groupID.Ref(), content)
		if err != nil {
			return errors.Wrapf(err, "publish call failed.")
		}
		log.Log("event", "publishTo", "type", "raw")
		goon.Dump(reply)
		return nil
	},
}

var groupsJoinCmd = &cli.Command{
	Name:   "join",
	Usage:  "manually join a group by adding the group key",
	Action: todo,
}

var inviteCmds = &cli.Command{
	Name: "invite",
	Subcommands: []*cli.Command{
		inviteCreateCmd,
		inviteAcceptCmd,
	},
}

var inviteCreateCmd = &cli.Command{
	Name:  "create",
	Usage: "register and return an invite for somebody else to accept",
	Flags: []cli.Flag{
		&cli.UintFlag{Name: "uses", Value: 1, Usage: "How many times an invite can be used"},
	},
	Action: func(ctx *cli.Context) error {

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var args legacyinvites.CreateArguments
		args.Uses = ctx.Uint("uses")

		var code string
		err = client.Async(longctx, &code, muxrpc.TypeString, muxrpc.Method{"invite", "create"}, args)
		if err != nil {
			return err
		}
		fmt.Println(code)
		return nil
	},
}

var inviteAcceptCmd = &cli.Command{
	Name:   "accept",
	Usage:  "use an invite code",
	Action: todo,
}

/* EBT HACKZ
var tryEBTCmd = &cli.Command{
	Name: "ebt",
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var opt = map[string]interface{}{
			"version": 2,
		}
		var msgs map[string]interface{}
		src, snk, err := client.Duplex(longctx, msgs, muxrpc.Method{"ebt", "replicate"}, opt)
		log.Log("event", "call replicate", "err", err)
		if err != nil {
			return err
		}
		go pump(longctx, snk)
		drain(longctx, src)
		// snk.Close()
		// <-longctx.Done()
		return nil
	},
}

func pump(ctx context.Context, snk luigi.Sink) {
	for i := 0; i < 10; i++ {
		err := snk.Pour(ctx, map[string]interface{}{
			"@k53z9zrXEsxytIE+38qaApl44ZJS68XvkepQ0fyJLdg=.ed25519": 100 + i,
			"@vZeAPvi6rMuB0bbGCciXzqJEmORnGt4rojCWVmOYXXU=.ed25519": 0,
		})
		log.Log("event", "pump done", "err", err, "i", i)
		time.Sleep(10 * time.Second)
	}
	snk.Close()
}

func drain(ctx context.Context, src luigi.Source) {
	snk := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}
		log.Log("event", "drain got")
		goon.Dump(val)
		return nil
	})
	err := luigi.Pump(ctx, snk, src)
	log.Log("event", "drain done", "err", err)
}
*/
