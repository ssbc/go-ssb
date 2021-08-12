// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"os"

	"go.cryptoscope.co/muxrpc/v2"
	"gopkg.in/urfave/cli.v2"

	"go.cryptoscope.co/ssb/plugins/friends"
	refs "go.mindeco.de/ssb-refs"
)

var friendsCmd = &cli.Command{
	Name: "friends",
	Subcommands: []*cli.Command{
		friendsIsFollowingCmd,
		friendsBlocksCmd,
		friendsHopsCmd,
	},
}

var friendsIsFollowingCmd = &cli.Command{
	Name: "isFollowing",
	Action: func(ctx *cli.Context) error {
		src := ctx.Args().Get(0)
		if src == "" {
			return errors.New("friends.isFollowing: needs source as param 1")
		}

		dst := ctx.Args().Get(1)
		if dst == "" {
			return errors.New("friends.isFollowing: needs dest as param 2")
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		srcRef, err := refs.ParseFeedRef(src)
		if err != nil {
			return err
		}

		dstRef, err := refs.ParseFeedRef(dst)
		if err != nil {
			return err
		}

		var arg = struct {
			Source refs.FeedRef `json:"source"`
			Dest   refs.FeedRef `json:"dest"`
		}{Source: srcRef, Dest: dstRef}

		var is bool
		err = client.Async(longctx, &is, muxrpc.TypeJSON, muxrpc.Method{"friends", "isFollowing"}, arg)
		if err != nil {
			return fmt.Errorf("connect: async call failed: %w", err)
		}

		log.Log("event", "friends.isFollowing", "is", is)
		return nil
	},
}
var friendsHopsCmd = &cli.Command{
	Name: "hops",
	Flags: []cli.Flag{
		&cli.UintFlag{Name: "dist", Value: 2, Usage: "non-remote repo allows for access withut involving a bot"},
	},
	Action: func(ctx *cli.Context) error {
		var arg friends.HopsArgs

		arg.Max = ctx.Uint("dist")

		if who := ctx.Args().Get(0); who != "" {
			startRef, err := refs.ParseFeedRef(who)
			if err != nil {
				return err
			}
			arg.Start = &startRef
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"friends", "hops"}, arg)
		if err != nil {
			return err
		}

		err = jsonDrain(os.Stdout, src)

		log.Log("done", err)
		return err
	},
}

var friendsBlocksCmd = &cli.Command{
	Name: "blocks",
	Action: func(ctx *cli.Context) error {
		var args = []interface{}{}

		if who := ctx.Args().Get(0); who != "" {
			args = append(args, struct {
				Who string
			}{who})
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		src, err := client.Source(longctx, muxrpc.TypeJSON, muxrpc.Method{"friends", "blocks"}, args...)
		if err != nil {
			return err
		}

		err = jsonDrain(os.Stdout, src)
		log.Log("done", err)
		return err
	},
}
