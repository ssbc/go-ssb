// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"go.cryptoscope.co/ssb/message"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/v2"
	refs "go.mindeco.de/ssb-refs"
	cli "gopkg.in/urfave/cli.v2"
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
	var ref *refs.FeedRef
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
	args.Gt = ctx.Int64("gt")
	args.Lt = ctx.Int64("lt")
	args.Reverse = ctx.Bool("reverse")
	args.Live = ctx.Bool("live")
	args.Keys = ctx.Bool("keys")
	args.Values = ctx.Bool("values")
	args.Private = ctx.Bool("private")
	return args
}

type mapMsg map[string]interface{}

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

		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"partialReplication", "getMessagesOfType"}, struct {
			ID   string `json:"id"`
			Tipe string `json:"type"`
		}{
			ID:   id,
			Tipe: ctx.Args().First(),
		})
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "byType failed")
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
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"createHistoryStream"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "feed hist failed")
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
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"createLogStream"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "log failed")
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
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"createFeedStream"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "log failed")
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
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"messagesByType"}, targs)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "byType failed")
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

		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"tangles", "read"}, targs)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "private/read failed")
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
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"replicate", "upto"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "replicate/upto failed")
	},
}

func jsonDrain(w io.Writer) luigi.Sink {
	i := 0
	return luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if luigi.IsEOS(err) {
			return nil
		} else if err != nil {
			return errors.Wrapf(err, "jsonDrain: failed to drain message %d", i)
		}
		b, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "jsonDrain: failed to encode msg %d", i)
		}
		_, err = fmt.Fprintln(w, string(b))
		if err != nil {
			return errors.Wrapf(err, "jsonDrain: failed to write msg %d", i)
		}
		i++
		return nil
	})
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
