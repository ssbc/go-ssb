package main

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
	goon "github.com/shurcooL/go-goon"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	cli "gopkg.in/urfave/cli.v2"
)

func publishRawCmd(ctx *cli.Context) error {
	var content interface{}
	err := json.NewDecoder(os.Stdin).Decode(&content)
	if err != nil {
		return errors.Wrapf(err, "publish/raw: invalid json input from stdin")
	}
	type reply map[string]interface{}
	v, err := client.Async(longctx, reply{}, muxrpc.Method{"publish"}, content)
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}
	log.Log("event", "published", "type", "raw")
	goon.Dump(v)
	return nil
}

func publishPostCmd(ctx *cli.Context) error {
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
	type reply map[string]interface{}
	var v interface{}
	var err error
	if recps := ctx.StringSlice("recps"); len(recps) > 0 {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"private", "publish"}, arg, recps)
	} else {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"publish"}, arg)
	}
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}

	log.Log("event", "published", "type", "post")
	goon.Dump(v)
	return nil
}

func publishVoteCmd(ctx *cli.Context) error {
	mref, err := ssb.ParseMessageRef(ctx.Args().First())
	if err != nil {
		return errors.Wrapf(err, "publish/vote: invalid msg ref")
	}

	arg := map[string]interface{}{
		"vote": map[string]interface{}{
			"link":       mref.Ref(),
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

	type reply map[string]interface{}
	var v interface{}
	if recps := ctx.StringSlice("recps"); len(recps) > 0 {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"private", "publish"}, arg, recps)
	} else {
		v, err = client.Async(longctx, reply{},
			muxrpc.Method{"publish"}, arg)
	}
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}

	log.Log("event", "published", "type", "vote")
	goon.Dump(v)
	return nil
}

func publishAboutCmd(ctx *cli.Context) error {
	aboutRef, err := ssb.ParseFeedRef(ctx.Args().First())
	if err != nil {
		return errors.Wrapf(err, "publish/about: invalid feed ref")
	}
	arg := map[string]interface{}{
		"about": aboutRef.Ref(),
		"type":  "about",
	}
	if n := ctx.String("name"); n != "" {
		arg["name"] = n
	}
	if img := ctx.String("image"); img != "" {
		blobRef, err := ssb.ParseBlobRef(img)
		if err != nil {
			return errors.Wrapf(err, "publish/about: invalid blob ref")
		}
		arg["image"] = blobRef
	}
	type reply map[string]interface{}
	v, err := client.Async(longctx, reply{}, muxrpc.Method{"publish"}, arg)
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}
	log.Log("event", "published", "type", "about")
	goon.Dump(v)
	return nil
}

func publishContactCmd(ctx *cli.Context) error {
	cref, err := ssb.ParseFeedRef(ctx.Args().First())
	if err != nil {
		return errors.Wrapf(err, "publish/contact: invalid feed ref")
	}
	if ctx.Bool("following") && ctx.Bool("blocking") {
		return errors.Errorf("publish/contact: can't be both true")
	}
	arg := map[string]interface{}{
		"contact":   cref.Ref(),
		"type":      "contact",
		"following": ctx.Bool("following"),
		"blocking":  ctx.Bool("blocking"),
	}
	type reply map[string]interface{}
	v, err := client.Async(longctx, reply{}, muxrpc.Method{"publish"}, arg)
	if err != nil {
		return errors.Wrapf(err, "publish call failed.")
	}
	log.Log("event", "published", "type", "contact")
	goon.Dump(v)
	return nil
}
