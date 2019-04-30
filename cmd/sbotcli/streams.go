package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/errors"
	goon "github.com/shurcooL/go-goon"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	cli "gopkg.in/urfave/cli.v2"
)

func historyStreamCmd(ctx *cli.Context) error {
	var args = getStreamArgs(ctx)
	src, err := client.Source(longctx, map[string]interface{}{}, muxrpc.Method{"createHistoryStream"}, args)
	if err != nil {
		return errors.Wrap(err, "source stream call failed")
	}

	for {
		v, err := src.Next(longctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "creatHist(%s): failed to drain", args.Id)
		}
		goon.Dump(v)
	}
	return nil
}

func logStreamCmd(ctx *cli.Context) error {
	var args = getStreamArgs(ctx)
	src, err := client.Source(longctx, map[string]interface{}{}, muxrpc.Method{"createLogStream"}, args)
	if err != nil {
		return errors.Wrap(err, "source stream call failed")
	}

	i := 0
	for {
		v, err := src.Next(longctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "createLogStream: failed to drain")
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "failed to encode msg %d", i)
		}
		_, err = fmt.Fprintln(os.Stdout, string(b))
		if err != nil {
			return errors.Wrapf(err, "failed to write msg %d", i)
		}
		i++
	}
	return nil
}

func privateReadCmd(ctx *cli.Context) error {
	var args = getStreamArgs(ctx)
	src, err := client.Source(longctx, map[string]interface{}{}, muxrpc.Method{"private", "read"}, args)
	if err != nil {
		return errors.Wrap(err, "source stream call failed")
	}
	for {
		v, err := src.Next(longctx)
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			return errors.Wrapf(err, "createLogStream: failed to drain")
		}
		goon.Dump(v)
	}
	return nil
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
