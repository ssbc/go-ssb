// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ssbc/go-muxrpc/v2"
	cli "github.com/urfave/cli/v2"

	refs "github.com/ssbc/go-ssb-refs"
)

var publishCmd = &cli.Command{
	Name:  "publish",
	Usage: "Publish a message by type (raw, post, about, contact and vote)",
	Subcommands: []*cli.Command{
		publishRawCmd,
		publishPostCmd,
		publishAboutCmd,
		publishContactCmd,
		publishVoteCmd,
	},
}

var publishRawCmd = &cli.Command{
	Name:        "raw",
	Usage:       "Read JSON from stdin and publish it as the content of a new message",
  ArgsUsage:   "<json>",
	Description: `Read JSON from stdin and publish it as the content of a new message.

Example:

    echo '{"type":"post","text":"example"}' | sbotcli publish raw`,
  
	// TODO: add private
	Action: func(ctx *cli.Context) error {
		var content interface{}
		err := json.NewDecoder(os.Stdin).Decode(&content)
		if err != nil {
			return fmt.Errorf("publish/raw: invalid json input from stdin: %w", err)
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var v string
		err = client.Async(longctx, &v, muxrpc.TypeString, muxrpc.Method{"publish"}, content)
		if err != nil {
			return fmt.Errorf("publish call failed: %w", err)
		}
		newMsg, err := refs.ParseMessageRef(v)
		if err != nil {
			return err
		}
		log.Log("event", "published", "type", "raw", "ref", newMsg.String())
		fmt.Fprintln(os.Stdout, newMsg.String())
		return nil
	},
}

var publishPostCmd = &cli.Command{
	Name:        "post",
	Usage:       "Publish a post (public or private)",
	ArgsUsage:   "<text>",
	Description: `Publish a post (public or private).

Example:

    sbotcli publish post "Gophers create a network of tunnel systems that provide protection."`,

	Flags: []cli.Flag{
		&cli.StringFlag{Name: "root", Value: "", Usage: "The key of the first message of the thread"},
		// TODO: Slice of branches
		&cli.StringFlag{Name: "branch", Value: "", Usage: "The key of the post being replied to"},
		&cli.StringSliceFlag{Name: "recps", Usage: "The public key(s) to whom this post will be published as a private message"},
	},
	Action: func(ctx *cli.Context) error {
		arg := map[string]interface{}{
			"text": ctx.Args().First(),
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

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var v string
		if recps := ctx.StringSlice("recps"); len(recps) > 0 {
			err = client.Async(longctx, &v,
				muxrpc.TypeString,
				muxrpc.Method{"private", "publish"}, arg, recps)
		} else {
			err = client.Async(longctx, &v,
				muxrpc.TypeString,
				muxrpc.Method{"publish"}, arg)
		}
		if err != nil {
			return fmt.Errorf("publish call failed: %w", err)
		}

		newMsg, err := refs.ParseMessageRef(v)
		if err != nil {
			return err
		}
		log.Log("event", "published", "type", "post", "ref", newMsg.String())
		fmt.Fprintln(os.Stdout, newMsg.String())
		return nil
	},
}

var publishVoteCmd = &cli.Command{
	Name:        "vote",
	Usage:       "Publish a vote or expression",
	ArgsUsage:   "<%...sha256>",
	Description: `Publish a vote or expression.

Example:

    sbotcli publish vote --value 1 %GOmisAlROznJEWJ/eht6tJWXgjFdQhbUKx4+/SeyoqI=.sha256`,

	Flags: []cli.Flag{
		&cli.IntFlag{Name: "value", Usage: "Usually 1 (like) or 0 (unlike)"},
		&cli.StringFlag{Name: "expression", Usage: "An expression made in response to a message (ie. 'dig', 'yup' or 'heart')"},

		&cli.StringFlag{Name: "root", Value: "", Usage: "The key of the first message of the thread"},
		// TODO: Slice of branches
		&cli.StringFlag{Name: "branch", Value: "", Usage: "The key of the post being replied to"},
		&cli.StringSliceFlag{Name: "recps", Usage: "The public key(s) to whom this post will be published as a private message"},
	},
	Action: func(ctx *cli.Context) error {
		mref, err := refs.ParseMessageRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("publish/vote: invalid msg ref: %w", err)
		}

		arg := map[string]interface{}{
			"vote": map[string]interface{}{
				"link":       mref.String(),
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

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var v string
		if recps := ctx.StringSlice("recps"); len(recps) > 0 {
			err = client.Async(longctx, &v,
				muxrpc.TypeString,
				muxrpc.Method{"private", "publish"}, arg, recps)
		} else {
			err = client.Async(longctx, &v,
				muxrpc.TypeString,
				muxrpc.Method{"publish"}, arg)
		}
		if err != nil {
			return fmt.Errorf("publish call failed: %w", err)
		}

		newMsg, err := refs.ParseMessageRef(v)
		if err != nil {
			return err
		}

		log.Log("event", "published", "type", "vote", "ref", newMsg.String())
		fmt.Fprintln(os.Stdout, newMsg.String())
		return nil
	},
}

var publishAboutCmd = &cli.Command{
	Name:        "about",
	Usage:       "Publish an about message to define the name or image assigned to a public key",
	ArgsUsage:   "<@...ed25519>",
	Description: `Publish an about message to define the name or image assigned to a public key.

Example:

    sbotcli publish about --name "glf" @r6Lzb9OT3/dlVYNDTABmsF+HWnhBsA1twZaobYhjVUY=.ed25519`,

	Flags: []cli.Flag{
		&cli.StringFlag{Name: "name", Usage: "The name to be assigned to the public key"},
		&cli.StringFlag{Name: "image", Usage: "The image blob ref to be assigned to the public key"},
	},
	Action: func(ctx *cli.Context) error {
		aboutRef, err := refs.ParseFeedRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("publish/about: invalid feed ref: %w", err)
		}
		arg := map[string]interface{}{
			"about": aboutRef.String(),
			"type":  "about",
		}
		if n := ctx.String("name"); n != "" {
			arg["name"] = n
		}
		if img := ctx.String("image"); img != "" {
			blobRef, err := refs.ParseBlobRef(img)
			if err != nil {
				return fmt.Errorf("publish/about: invalid blob ref: %w", err)
			}
			arg["image"] = blobRef
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var v string
		err = client.Async(longctx, &v, muxrpc.TypeString, muxrpc.Method{"publish"}, arg)
		if err != nil {
			return fmt.Errorf("publish call failed: %w", err)
		}
		newMsg, err := refs.ParseMessageRef(v)
		if err != nil {
			return err
		}
		log.Log("event", "published", "type", "about", "ref", newMsg.String())
		fmt.Fprintln(os.Stdout, newMsg.String())
		return nil
	},
}

var publishContactCmd = &cli.Command{
	Name:        "contact",
	Usage:       "Publish a contact message (follow, unfollow, block, unblock)",
	ArgsUsage:   "<@...ed25519>",
	Description: `Publish a contact message (follow, unfollow, block, unblock).

Example (follow):

    sbotcli publish contact --following @r6Lzb9OT3/dlVYNDTABmsF+HWnhBsA1twZaobYhjVUY=.ed25519

Example (unfollow):

    sbotcli publish contact @r6Lzb9OT3/dlVYNDTABmsF+HWnhBsA1twZaobYhjVUY=.ed25519

Example (block):

    sbotcli publish contact --blocking @r6Lzb9OT3/dlVYNDTABmsF+HWnhBsA1twZaobYhjVUY=.ed25519

Note that both fields (following and blocking) are set with each published contact message.
This means that both the following and blocking fields can be updated with a single published message.`,

	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "following", Usage: "A boolean expression denoting the follow state"},
		&cli.BoolFlag{Name: "blocking", Usage: "A boolean expression denoting the block state"},

		&cli.StringSliceFlag{Name: "recps", Usage: "The public key(s) to whom this post will be published as a private message"},
	},
	Action: func(ctx *cli.Context) error {
		cref, err := refs.ParseFeedRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("publish/contact: invalid feed ref: %w", err)
		}
		if ctx.Bool("following") && ctx.Bool("blocking") {
			return fmt.Errorf("publish/contact: can't be both true")
		}
		arg := map[string]interface{}{
			"contact":   cref.String(),
			"type":      "contact",
			"following": ctx.Bool("following"),
			"blocking":  ctx.Bool("blocking"),
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var v string
		err = client.Async(longctx, &v, muxrpc.TypeString, muxrpc.Method{"publish"}, arg)
		if err != nil {
			return fmt.Errorf("publish call failed: %w", err)
		}

		newMsg, err := refs.ParseMessageRef(v)
		if err != nil {
			return err
		}
		log.Log("event", "published", "type", "contact", "ref", newMsg.String())
		fmt.Fprintln(os.Stdout, newMsg.String())
		return nil
	},
}
