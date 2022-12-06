// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ssbc/go-muxrpc/v2"
)

type namedPlugin struct {
	h    muxrpc.Handler
	name string
}

func (np namedPlugin) Name() string { return np.name }

func (np namedPlugin) Method() muxrpc.Method {
	return muxrpc.Method{np.name}
}

func (np namedPlugin) Handler() muxrpc.Handler {
	return np.h
}

type manifestHandler string

func (manifestHandler) Handled(m muxrpc.Method) bool { return m.String() == "manifest" }

func (manifestHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h manifestHandler) HandleCall(ctx context.Context, req *muxrpc.Request) {
	err := req.Return(ctx, json.RawMessage(h))
	if err != nil {
		fmt.Println("manifest err", err)
	}
}

func init() {
	manifestMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(manifestBlob), &manifestMap)
	if !json.Valid([]byte(manifestBlob)) || err != nil {
		fmt.Println(err)
		panic("manifestBlob is broken json")
	}

	// remove the whitespaces
	condensed, err := json.Marshal(manifestMap)
	if err != nil {
		panic(err)
	}
	manifestBlob = manifestHandler(condensed)
}

// hardcoded manifest for MUXRPC clients
var manifestBlob manifestHandler = `
{
	"blobs": {
		"add": "sink",
		"createWants": "source",
		"get": "source",
		"has": "async",
		"size": "async",
		"want": "async"
	},
	"conn": {
		"connect": "async",
		"dialViaRoom": "async",
		"disconnect": "async",
		"replicate": "async"
	},
	"createFeedStream": "source",
	"createHistoryStream": "source",
	"createLogStream": "source",
	"ebt": {
		"replicate": "duplex"
	},
	"friends": {
		"blocks": "source",
		"hops": "source",
		"isBlocking": "async",
		"isFollowing": "async"
	},
	"get": "async",
	"gossip": {
		"connect": "async",
		"ping": "duplex"
	},
	"groups": {
		"create":"async",
		"publishTo":"async"
  },
	"invite": {
		"create": "async",
		"use": "async"
	},
	"manifest": "sync",
	"messagesByType": "source",
	"names": {
		"get": "async",
		"getImageFor": "async",
		"getSignifier": "async"
	},
	"partialReplication": {
		"getSubset": "source",
		"getTangle": "async"
	},
	"publish": "async",
	"private": {
		"publish": "async",
		"read":"source"
	},
	"replicate": {
		"upto": "source"
	},
	"status": "sync",
	"tangles": {
		"thread": "source"
	},
	"tunnel": {
		"connect": "duplex",
		"isRoom": "async",
		"ping": "async"
	},
	"whoami": "sync"
}
`
