// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/ssbc/go-muxrpc/v2"
	"github.com/urfave/cli/v2"

	"github.com/ssbc/go-ssb"
	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb/internal/aliases"
)

var aliasCmd = &cli.Command{
	Name:  "alias",
	Usage: "Register and revoke user aliases (for use with SSB Room servers)",
	Subcommands: []*cli.Command{
		aliasRegisterCmd,
		aliasRevokeCmd,
	},
}

var aliasRegisterCmd = &cli.Command{
	Name:      "register",
	Usage:     "Register a new alias on the remote room (should be used with --remotekey and --addr)",
	ArgsUsage: "<alias>",
	Action: func(ctx *cli.Context) error {

		alias := ctx.Args().Get(0)
		if alias == "" {
			return errors.New("alias.register: need a name to register")
		}

		localKey, err := ssb.LoadKeyPair(ctx.String("key"))
		if err != nil {
			return err
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		roomID, err := refs.ParseFeedRef(ctx.String("remotekey"))
		if err != nil {
			return err
		}

		var reg aliases.Registration
		reg.Alias = alias
		reg.UserID = localKey.ID()
		reg.RoomID = roomID

		conf := reg.Sign(localKey.Secret())
		sig := base64.StdEncoding.EncodeToString(conf.Signature) + ".sig.ed25519"

		var ok bool
		method := muxrpc.Method{"room", "registerAlias"}
		err = client.Async(longctx, &ok, muxrpc.TypeJSON, method, alias, sig)
		if err != nil {
			return fmt.Errorf("alias.register: async call failed: %w", err)
		}
		log.Log("event", "alias registerd", "ok", ok)
		return nil
	},
}

var aliasRevokeCmd = &cli.Command{
	Name:      "revoke",
	Usage:     "Removes the alias from the remote (should be used with --remotekey and --addr)",
	ArgsUsage: "<%...sha256>",
	Action: func(ctx *cli.Context) error {
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("register: need a blob ref")
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		method := muxrpc.Method{"room", "revoke"}

		var has bool
		err = client.Async(longctx, &has, muxrpc.TypeJSON, method, ref)
		if err != nil {
			return fmt.Errorf("connect: async call failed: %w", err)
		}
		log.Log("event", "blob.has", "r", has)

		if !has {
			log.Log("blob.has", false)
			os.Exit(1)
			return nil
		}
		return nil
	},
}
