// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ssbc/go-muxrpc/v2"
	"github.com/urfave/cli/v2"

	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb/plugins/friends"
)

var friendsCmd = &cli.Command{
	Name:  "friends",
	Usage: "Retrieve information about the social graph (follows, blocks, hops)",
	Subcommands: []*cli.Command{
		friendsIsFollowingCmd,
		friendsBlocksCmd,
		friendsHopsCmd,
	},
}

var friendsIsFollowingCmd = &cli.Command{
	Name:      "isFollowing",
	Usage:     "Check if a feed is friends with another (follows back)",
	ArgsUsage: "<@...ed25519> <@...ed25519>",
	Description: `Check if a feed is friends with another (follows back).

Example:

sbotcli friends isFollowing @HEqy940T6uB+T+d9Jaa58aNfRzLx9eRWqkZljBmnkmk=.ed25519 @mfY4X9Gob0w2oVfFv+CpX56PfL0GZ2RNQkc51SJlMvc=.ed25519`,

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
	Name:      "hops",
	Usage:     "List all peers within the hops range of the given feed ID",
	ArgsUsage: "<@...ed25519>",
	Description: `List all peers within the hops range of the given feed ID.

When the <dist> flag is set to 0, the peers in the returned list represent the
friends (mutual follows) of the given feed ID. Setting <dist> to 1 includes
friends-of-friends in the output. The default value is 2. Be aware that this
may return a long list!

Example:

    sbotcli friends hops --dist 0 @HEqy940T6uB+T+d9Jaa58aNfRzLx9eRWqkZljBmnkmk=.ed25519`,
	Flags: []cli.Flag{
		&cli.UintFlag{Name: "dist", Value: 2, Usage: "Hops range"},
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
	Name:      "blocks",
	Usage:     "List all peers blocked by the given feed ID",
	ArgsUsage: "<@...ed25519>",
	Description: `List all peers blocked by the given feed ID.

Example:

    sbotcli friends blocks @fGWzOR/FXU3Acbn4P65CpMewJIynFyqocvfLAyJdDno=.ed25519`,

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
