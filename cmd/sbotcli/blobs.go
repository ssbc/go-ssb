// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"go.cryptoscope.co/muxrpc/v2"
	"gopkg.in/urfave/cli.v2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	refs "go.mindeco.de/ssb-refs"
)

var blobsStore ssb.BlobStore

var blobsCmd = &cli.Command{
	Name: "blobs",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "localstore", Value: "", Usage: "non-remote repo allows for access withut involving a bot"},
	},
	Before: func(ctx *cli.Context) error {
		var localRepo = ctx.String("localstore")
		if localRepo == "" {
			//blobsStore, err = newClient(ctx)
			return fmt.Errorf("TODO: implement more blobs features on client")
		}
		var err error
		blobsStore, err = blobstore.New(localRepo)
		if err != nil {
			return fmt.Errorf("blobs: failed to construct local edp: %w", err)
		}
		log.Log("before", "blobs", "locl")
		return nil
	},
	Subcommands: []*cli.Command{
		blobsHasCmd,
		blobsWantCmd,
		blobsAddCmd,
		blobsGetCmd,
	},
}

var blobsHasCmd = &cli.Command{
	Name:  "has",
	Usage: "check if a blob is in the repo",
	Action: func(ctx *cli.Context) error {
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.has: need a blob ref")
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		// TODO: direct blobstore mode!?
		var has bool
		//sz, err := blobsStore.Size()
		err = client.Async(longctx, &has, muxrpc.TypeJSON, muxrpc.Method{"blobs", "has"}, ref)
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

var blobsWantCmd = &cli.Command{
	Name:  "want",
	Usage: "try to get it from other peers",
	Action: func(ctx *cli.Context) error {
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.want: need a blob ref")
		}
		br, err := refs.ParseBlobRef(ref)
		if err != nil {
			return fmt.Errorf("blobs: failed to parse argument ref: %w", err)
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		return client.BlobsWant(br)
	},
}

var blobsAddCmd = &cli.Command{
	Name:  "add",
	Usage: "add a file to the store (use - to open stdin)",
	Action: func(ctx *cli.Context) error {
		if blobsStore == nil {
			return fmt.Errorf("no blobstore use 'blobs --localstore $repo/blobs add -' for now")
		}
		fname := ctx.Args().Get(0)
		if fname == "" {
			return errors.New("blobs.add: need file to add (- for stdin)")
		}

		var rd io.Reader
		if fname == "-" {
			rd = os.Stdin
		} else {
			var err error
			rd, err = os.Open(fname)
			if err != nil {
				return fmt.Errorf("blobs.add: failed to open input file: %w", err)
			}
		}

		ref, err := blobsStore.Put(rd)
		log.Log("blobs.add", ref.Sigil())
		return err
	},
}

var blobsGetCmd = &cli.Command{
	Name:  "get",
	Usage: "prints the first argument to stdout",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "out", Value: "-", Usage: "where to? (stdout by default)"},
	},
	Action: func(ctx *cli.Context) error {
		if blobsStore == nil {
			return fmt.Errorf("no blobstore use 'blobs --localstore $repo/blobs get &...' for now")
		}
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.get: need a blob ref")
		}
		br, err := refs.ParseBlobRef(ref)
		if err != nil {
			return fmt.Errorf("blobs: failed to parse argument ref: %w", err)
		}
		rd, err := blobsStore.Get(br)
		if err != nil {
			return fmt.Errorf("blobs: failed to parse argument ref: %w", err)
		}

		var out io.Writer
		outName := ctx.String("out")
		if outName == "-" {
			out = os.Stdout
		} else {
			var err error
			out, err = os.Create(outName)
			if err != nil {
				return fmt.Errorf("blobs.get: failed to open output file: %w", err)
			}
		}

		n, err := io.Copy(out, rd)
		log.Log("blobs.get", br.Sigil(), "written", n)
		return err
	},
}
