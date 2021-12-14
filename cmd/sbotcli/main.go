// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// sbotcli implements a simple tool to query commands on another sbot
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	goon "github.com/shurcooL/go-goon"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/ssb/query"
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
		getSubsetCmd,
		inviteCmds,
		logStreamCmd,
		sortedStreamCmd,
		typeStreamCmd,
		historyStreamCmd,
		replicateUptoCmd,
		repliesStreamCmd,
		callCmd,
		sourceCmd,
		connectCmd,
		publishCmd,
		groupsCmd,
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
			level.Warn(log).Log("client", "unix-path based init failed", "err", err)
			return newTCPClient(ctx)
		}
		level.Info(log).Log("client", "connected", "method", "unix sock")
		return client, nil
	}

	// Assume TCP connection
	return newTCPClient(ctx)
}

func newTCPClient(ctx *cli.Context) (*ssbClient.Client, error) {
	localKey, err := ssb.LoadKeyPair(ctx.String("key"))
	if err != nil {
		return nil, err
	}

	var remotePubKey = make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(remotePubKey, localKey.ID().PubKey())
	if rk := ctx.String("remoteKey"); rk != "" {
		rk = strings.TrimSuffix(rk, ".ed25519")
		rk = strings.TrimPrefix(rk, "@")
		rpk, err := base64.StdEncoding.DecodeString(rk)
		if err != nil {
			return nil, fmt.Errorf("init: base64 decode of --remoteKey failed: %w", err)
		}
		copy(remotePubKey, rpk)
	}

	plainAddr, err := net.ResolveTCPAddr("tcp", ctx.String("addr"))
	if err != nil {
		return nil, fmt.Errorf("int: failed to resolve TCP address: %w", err)
	}

	shsAddr := netwrap.WrapAddr(plainAddr, secretstream.Addr{PubKey: remotePubKey})
	client, err := ssbClient.NewTCP(localKey, shsAddr,
		ssbClient.WithSHSAppKey(ctx.String("shscap")),
		ssbClient.WithContext(longctx))
	if err != nil {
		return nil, fmt.Errorf("init: failed to connect to %s: %w", shsAddr.String(), err)
	}
	level.Info(log).Log("client", "connected", "method", "tcp")
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
		method := strings.Split(cmd, ".")

		args := ctx.Args().Slice()
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
		err = client.Async(longctx, &reply, muxrpc.TypeJSON, muxrpc.Method(method), sendArgs...)
		if err != nil {
			return fmt.Errorf("%s: call failed: %w", cmd, err)
		}
		level.Debug(log).Log("event", "call reply")

		jsonReply, err := json.MarshalIndent(reply, "", "  ")
		if err != nil {
			return fmt.Errorf("%s: indent failed: %w", cmd, err)
		}

		_, err = os.Stdout.Write(jsonReply)
		if err != nil {
			return fmt.Errorf("%s: result copy failed: %w", cmd, err)
		}

		return nil
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
			ID    string `json:"id,omitempty"`
			Limit int    `json:"limit"`
		}{
			ID:    ctx.String("id"),
			Limit: ctx.Int("limit"),
		}

		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method(v), args)
		if err != nil {
			return fmt.Errorf("%s: call failed: %w", cmd, err)
		}
		level.Debug(log).Log("event", "call reply")

		err = jsonDrain(os.Stdout, src)
		return fmt.Errorf("%s: result copy failed: %w", cmd, err)
	},
}

var getSubsetCmd = &cli.Command{
	Name:  "subset",
	Usage: "invoke the partialReplication.getSubset muxrpc",

	// define cli flags
	Flags: []cli.Flag{
		&cli.IntFlag{Name: "limit", Value: -1},
		&cli.BoolFlag{Name: "dsc", Value: false},
		&cli.BoolFlag{Name: "keys", Value: false},
	},

	Action: func(ctx *cli.Context) error {
		if ctx.NArg() < 1 {
			return errors.New(`subset usage: sbotcli subset [optional --flags] <valid json>`)
		}
		input := ctx.Args().Get(0)
		cmd := "subset"

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		// a helper struct borrowed from the query package for converting the string input into a something we can send over
		// the wire & which will be unpacked correctly
		var payload struct {
			Operation string `json:"op"`

			Args   []query.SubsetOperation `json:"args,omitempty"`
			String string            `json:"string,omitempty"`
			Feed   *refs.FeedRef     `json:"feed,omitempty"`
			// embed query.SubsetOptions (adds limit, dsc and keys)
			query.SubsetOptions
		}

		err = json.Unmarshal([]byte(input), &payload)
		if err != nil {
			return fmt.Errorf("failed to unmarshal input (%w)", err)
		}
		// set the options after unmarshaling the input into struct form
		payload.PageLimit = ctx.Int("limit")
		payload.Descending = ctx.Bool("dsc")
		payload.Keys = ctx.Bool("keys")

		method := muxrpc.Method(strings.Split("partialReplication.getSubset", "."))
		src, err := client.Source(longctx, muxrpc.TypeJSON, method, payload)
		if err != nil {
			return fmt.Errorf("%s call failed (%w)", cmd, err)
		}
		level.Debug(log).Log("event", "call reply")

		err = jsonDrain(os.Stdout, src)
		if err != nil {
			return fmt.Errorf("%s: result copy failed (%w)", cmd, err)
		}
		return nil
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
			ID      refs.MessageRef `json:"id"`
			Private bool            `json:"private"`
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

		// try all three of these
		var methods = []muxrpc.Method{
			{"conn", "connect"},   // latest, npm:ssb-conn version, now also supported by go-ssb
			{"gossip", "connect"}, // previous javascript call
			{"ctrl", "connect"},   // previous go-ssb version
		}

		var val string
		for _, m := range methods {
			err = client.Async(longctx, &val, muxrpc.TypeString, m, to)
			if err == nil {
				break
			}
			level.Warn(log).Log("event", "connect command failed", "err", err, "method", m.String())

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

		var blocked = make(map[string]bool)

		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fr, err := refs.ParseFeedRef(sc.Text())
			if err != nil {
				return err
			}
			blocked[fr.String()] = true
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

		if groupID.Algo() != refs.RefAlgoCloakedGroup {
			return fmt.Errorf("groupID needs to be a cloaked message ref, not %s", groupID.Algo())
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
		err = client.Async(longctx, &reply, muxrpc.TypeJSON, muxrpc.Method{"groups", "invite"}, groupID.String(), member.String())
		if err != nil {
			return fmt.Errorf("invite call failed: %w", err)
		}
		log.Log("event", "member added", "group", groupID.String(), "member", member.String())
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
			return fmt.Errorf("publish/raw: invalid json input from stdin: %w", err)
		}

		groupID, err := refs.ParseMessageRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("groupID needs to be a valid message ref: %w", err)
		}

		if groupID.Algo() != refs.RefAlgoCloakedGroup {
			return fmt.Errorf("groupID needs to be a cloaked message ref, not %s", groupID.Algo())
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var reply interface{}
		err = client.Async(longctx, &reply, muxrpc.TypeJSON, muxrpc.Method{"groups", "publishTo"}, groupID.String(), content)
		if err != nil {
			return fmt.Errorf("publish call failed: %w", err)
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
