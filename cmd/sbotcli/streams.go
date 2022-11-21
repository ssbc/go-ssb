// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ssbc/go-muxrpc/v2"
	cli "github.com/urfave/cli/v2"

	"github.com/ssbc/go-ssb/message"
	refs "github.com/ssbc/go-ssb-refs"
)

var streamFlags = []cli.Flag{
	&cli.IntFlag{Name: "limit", Value: -1},
	&cli.IntFlag{Name: "seq", Value: 0},
	&cli.IntFlag{Name: "gt"},
	&cli.IntFlag{Name: "lt"},
	&cli.BoolFlag{Name: "reverse"},
	&cli.BoolFlag{Name: "live"},
	&cli.BoolFlag{Name: "keys", Value: false},
	&cli.BoolFlag{Name: "values", Value: false},
	&cli.BoolFlag{Name: "private", Value: false},
}

func getStreamArgs(ctx *cli.Context) message.CreateHistArgs {
	var ref refs.FeedRef
	if id := ctx.String("id"); id != "" {
		var err error
		ref, err = refs.ParseFeedRef(id)
		if err != nil {
			panic(err)
		}
	}
	args := message.CreateHistArgs{
		ID:     ref,
		Seq:    ctx.Int64("seq"),
		AsJSON: ctx.Bool("asJSON"),
	}
	args.Limit = ctx.Int64("limit")
	args.Gt = message.RoundedInteger(ctx.Int64("gt"))
	args.Lt = message.RoundedInteger(ctx.Int64("lt"))
	args.Reverse = ctx.Bool("reverse")
	args.Live = ctx.Bool("live")
	args.Keys = ctx.Bool("keys")
	args.Values = ctx.Bool("values")
	args.Private = ctx.Bool("private")
	return args
}

var historyStreamCmd = &cli.Command{
	Name:  "hist",
	Usage: "Fetch all messages authored by the local keypair / author",
	Flags: append(streamFlags, &cli.StringFlag{Name: "id"}, &cli.BoolFlag{Name: "asJSON"}),
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		ir, err := client.Whoami()
		if err != nil {
			return err
		}

		var args = getStreamArgs(ctx)
		args.ID = ir
		if f := ctx.String("id"); f != "" {
			flagRef, err := refs.ParseFeedRef(f)
			if err != nil {
				return err
			}
			args.ID = flagRef
		}
		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"createHistoryStream"}, args)
		if err != nil {
			return fmt.Errorf("source stream call failed: %w", err)
		}
		err = jsonDrain(os.Stdout, src)
		if err != nil {
			err = fmt.Errorf("feed hist pump failed: %w", err)
		}
		return err
	},
}

var logStreamCmd = &cli.Command{
	Name:  "log",
	Usage: "Fetch all messages from the local database (ordered by received time)",
	Flags: streamFlags,
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var args = getStreamArgs(ctx)
		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"createLogStream"}, args)
		if err != nil {
			return fmt.Errorf("source stream call failed: %w", err)
		}
		err = jsonDrain(os.Stdout, src)
		if err != nil {
			err = fmt.Errorf("message pump failed: %w", err)
		}
		return err
	},
}

var sortedStreamCmd = &cli.Command{
	Name:  "sorted",
	Usage: "Fetch all messages from the local database (ordered by message timestamps)",
	Flags: streamFlags,
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var args = getStreamArgs(ctx)
		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"createFeedStream"}, args)
		if err != nil {
			return fmt.Errorf("source stream call failed: %w", err)
		}
		err = jsonDrain(os.Stdout, src)
		if err != nil {
			err = fmt.Errorf("message pump failed: %w", err)
		}
		return err
	},
}

var typeStreamCmd = &cli.Command{
	Name:  "bytype",
	Usage: "Fetch all messages from the local database matching the given type (e.g. post, vote, about etc.)",
	Flags: streamFlags,
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		var targs message.MessagesByTypeArgs
		arg := getStreamArgs(ctx)
		targs.CommonArgs = arg.CommonArgs
		targs.StreamArgs = arg.StreamArgs
		targs.Type = ctx.Args().First()
		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"messagesByType"}, targs)
		if err != nil {
			return fmt.Errorf("source stream call failed: %w", err)
		}
		err = jsonDrain(os.Stdout, src)
		if err != nil {
			err = fmt.Errorf("message pump failed: %w", err)
		}
		return err
	},
}

var repliesStreamCmd = &cli.Command{
	Name:  "replies",
	Usage: "Fetch all replies to the given root message (%...)",
	Flags: append(streamFlags, &cli.StringFlag{Name: "tname", Usage: "tangle name (v2)"}),
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var targs message.TanglesArgs
		arg := getStreamArgs(ctx)
		targs.CommonArgs = arg.CommonArgs
		targs.StreamArgs = arg.StreamArgs
		targs.Root, err = refs.ParseMessageRef(ctx.Args().First())
		if err != nil {
			return err
		}
		targs.Name = ctx.String("tname")

		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"tangles", "thread"}, targs)
		if err != nil {
			return fmt.Errorf("source stream call failed: %w", err)
		}
		err = jsonDrain(os.Stdout, src)
		if err != nil {
			err = fmt.Errorf("message pump failed: %w", err)
		}
		return err
	},
}

var replicateUptoCmd = &cli.Command{
	Name:  "upto",
	Usage: "Return a list of all public keys in the log, along with the latest sequence number for each",
	Flags: streamFlags,
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		var args = getStreamArgs(ctx)
		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"replicate", "upto"}, args)
		if err != nil {
			return fmt.Errorf("source stream call failed: %w", err)
		}
		err = jsonDrain(os.Stdout, src)
		if err != nil {
			err = fmt.Errorf("message pump failed: %w", err)
		}
		return err
	},
}

func jsonDrain(w io.Writer, r *muxrpc.ByteSource) error {

	var buf = &bytes.Buffer{}
	for r.Next(context.TODO()) { // read/write loop for messages

		buf.Reset()
		err := r.Reader(func(r io.Reader) error {
			_, err := buf.ReadFrom(r)
			return err
		})
		if err != nil {
			return err
		}

		// jsonReply, err := json.MarshalIndent(buf.Bytes(), "", "  ")
		// if err != nil {
		// 	return err
		// }

		_, err = buf.WriteTo(os.Stdout)
		if err != nil {
			return err
		}

	}
	return r.Err()
}
