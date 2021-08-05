// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"go.cryptoscope.co/muxrpc/v2"
	cli "gopkg.in/urfave/cli.v2"

	"go.cryptoscope.co/ssb/message"
	refs "go.mindeco.de/ssb-refs"
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

var partialStreamCmd = &cli.Command{
	Name:  "partial",
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
		id := ir.Ref()
		if f := ctx.String("id"); f != "" {
			id = f
		}

		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"partialReplication", "getMessagesOfType"}, struct {
			ID   string `json:"id"`
			Tipe string `json:"type"`
		}{
			ID:   id,
			Tipe: ctx.Args().First(),
		})
		if err != nil {
			return fmt.Errorf("source stream call failed: %w", err)
		}
		err = jsonDrain(os.Stdout, src)

		if err != nil {
			err = fmt.Errorf("byType pump failed: %w", err)
		}
		return err
	},
}

var historyStreamCmd = &cli.Command{
	Name:  "hist",
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

		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"tangles", "replies"}, targs)
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
