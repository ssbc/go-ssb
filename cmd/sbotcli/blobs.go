// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ssbc/go-muxrpc/v2"
	"github.com/urfave/cli/v2"

	"github.com/ssbc/go-ssb"
	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb/blobstore"
)

var blobsStore ssb.BlobStore

var blobsCmd = &cli.Command{
	Name:  "blobs",
	Usage: "Add a blob to the local store or call MUXRPC methods: `has`, `get` and `wants`",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "path", Value: "", Usage: "Specify the path to the blobs folder of the sbot you want to query"},
	},
	Before: func(ctx *cli.Context) error {
		var blobsDir = ctx.String("path")
		if blobsDir == "" {
			homedir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory (%w)", err)
			}
			blobsDir = filepath.Join(homedir, ".ssb-go", "blobs")
		}
		if _, err := os.Stat(blobsDir); os.IsNotExist(err) {
			return fmt.Errorf("folder %s did not exist (%w)", blobsDir, err)
		}

		var err error
		blobsStore, err = blobstore.New(blobsDir)
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
	Name:      "has",
	Usage:     "Check if a blob is in the local blobstore.",
	ArgsUsage: "<&...sha256>",
	Description: `Check if a blob is in the local blobstore.

Example:

    sbotcli blobs has "&hB2vsBGwqPAfkBQ5IQGIrLfHXzytmExYC3iJ6FC08F8=.sha256"`,

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
		var arr []bool
		//sz, err := blobsStore.Size()
		err = client.Async(longctx, &arr, muxrpc.TypeJSON, muxrpc.Method{"blobs", "has"}, ref)
		if err != nil {
			return fmt.Errorf("connect: async call failed: %w", err)
		}

		var has bool
		if len(arr) > 0 {
			has = arr[0]
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
	Name:      "want",
	Usage:     "Try to get a blob from other peers.",
	ArgsUsage: "<&...sha256>",
	Description: `Try to get a blob from other peers.

Adds the blob reference to a list of blobs being requested by the local sbot.
This list of desired blobs is then shared with connected peers who then
replicate the blob(s) if they have them. This means the requested blob(s) will
likely not be received straight away (ie. there may be replication delays).

Example:

    sbotcli blobs has "&hB2vsBGwqPAfkBQ5IQGIrLfHXzytmExYC3iJ6FC08F8=.sha256"`,

	Action: func(ctx *cli.Context) error {
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.want: need a blob ref")
		}
		blobsRef, err := refs.ParseBlobRef(ref)
		if err != nil {
			return fmt.Errorf("blobs: failed to parse argument ref: %w", err)
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		return client.BlobsWant(blobsRef)
	},
}

var blobsAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "Add a file to the blobstore (pass - to open stdin).",
	ArgsUsage: "[<filename> | - <stdin>]",
	Description: `Add a file to the blobstore (pass - to open stdin).

A blob reference will be returned (<&...sha256>) if the file is added successfully.

Example:

    cat /home/glyph/Pictures/2022/cabin_computer_setup.jpeg | sbotcli blobs add -`,

	Action: func(ctx *cli.Context) error {
		if blobsStore == nil {
			return fmt.Errorf("no blobstore use 'blobs --path $repo/blobs add -' for now")
		}
		fname := ctx.Args().Get(0)
		if fname == "" {
			return errors.New("blobs.add: need file to add (- for stdin)")
		}

		var reader io.Reader
		if fname == "-" {
			reader = os.Stdin
		} else {
			var err error
			reader, err = os.Open(fname)
			if err != nil {
				return fmt.Errorf("blobs.add: failed to open input file: %w", err)
			}
		}

		ref, err := blobsStore.Put(reader)
		log.Log("blobs.add", ref.Sigil())
		return err
	},
}

var blobsGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "Streams the contents of the file",
	ArgsUsage: "<&...sha256>",
	Description: `Streams the contents of the file.

Contents are streamed to stdout by default. An alternative destination can be
defined using the 'out' flag.

Example:

    sbotcli blobs get "&grLTZFapgHZHXRYh1zgz2bTuDelottGZSfogKauo/fk=.sha256" > blob_file`,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "out", Value: "-", Usage: "Where to? (stdout by default)"},
	},
	Action: func(ctx *cli.Context) error {
		if blobsStore == nil {
			return fmt.Errorf("no blobstore use 'blobs --path $repo/blobs get &...' for now")
		}
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.get: need a blob ref")
		}
		blobsRef, err := refs.ParseBlobRef(ref)
		if err != nil {
			return fmt.Errorf("blobs: failed to parse argument ref: %w", err)
		}
		reader, err := blobsStore.Get(blobsRef)
		if err != nil {
			return fmt.Errorf("blobs: failed to retrieve blob from store: %w", err)
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

		n, err := io.Copy(out, reader)
		log.Log("blobs.get", blobsRef.Sigil(), "written", n)
		return err
	},
}
