<!--
SPDX-FileCopyrightText: 2021 The Go-SSB Authors

SPDX-License-Identifier: MIT
-->

<h1 align="center">Go-SSB</h1>

<p align="center">
  <img height="170px" src="./docs/icon.png" alt="hermit gopher with a shell and crab hands">
</p>

[![GoDoc](https://godoc.org/github.com/ssbc/go-ssb?status.svg)](https://godoc.org/github.com/ssbc/go-ssb) [![Go Report Card](https://goreportcard.com/badge/github.com/ssbc/go-ssb)](https://goreportcard.com/report/github.com/ssbc/go-ssb) ![Github Actions](https://github.com/ssbc/go-ssb/actions/workflows/go.yml/badge.svg) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT) [![REUSE status](https://api.reuse.software/badge/github.com/ssbc/go-ssb)](https://api.reuse.software/info/github.com/ssbc/go-ssb)

> **WARNING**: Project is still in alpha, backwards incompatible changes will be made.

A full-stack implementation of [secure-scuttlebutt](https://www.scuttlebutt.nz) using the [Go](https://golang.org) programming language.

If you encounter a bug, please refer to our [public issue tracker](https://github.com/ssbc/go-ssb/issues).

See the [FAQ](./docs/faq.md) for more. See [this project](https://github.com/orgs/ssbc/projects/2) for current focus.

### Developing

Want to contribute patches to go-ssb? Read the [developer documentation](https://dev.scuttlebutt.nz/#/golang/) hosted at [dev.scuttlebutt.nz](https://dev.scuttlebutt.nz/#/golang/). If you have a large change you want to make, [reach out](#contact) first and we'll together make sure that the resulting PR will be accepted :black_heart:

## Server Features

* [x] Follow-graph implementation (based on [gonum](https://www.gonum.org)) to authorize incoming connections
* [x] [Blobs](https://ssbc.github.io/scuttlebutt-protocol-guide/#blobs) store and replication
* [x] Publishing new messages to the log
* [x] _Legacy_ feed [replication](https://ssbc.github.io/scuttlebutt-protocol-guide/#createHistoryStream)
* [x] [Epidemic Broadcast Trees (EBT)](https://github.com/dominictarr/epidemic-broadcast-trees) feed replication in beta (use `go-sbot -enable-ebt`)
* [x] Invite mechanics ([peer-invites](https://github.com/ssbc/ssb-peer-invites) partially done, too. See [Issue 45](https://github.com/ssbc/go-ssb/issues/45) for more.)

## Installation

You can install the project using Golang's [install command](https://golang.org/cmd/go/#hdr-Compile_and_install_packages_and_dependencies) which will place the commands into the directory pointed to by the GOBIN environment variable.

```bash
git clone https://github.com/ssbc/go-ssb
cd go-ssb
go install ./cmd/go-sbot
go install ./cmd/sbotcli
```

Requirements:

  - [Golang](https://www.golang.org) version 1.17 or higher

## Running `go-sbot`

The tool in `cmd/go-sbot` is similar to [ssb-server](https://github.com/ssbc/ssb-server) (previously called scuttlebot or sbot for short).

See the [quick start](./docs/quick-start.md) document for a walkthrough and getting started tour. You may also be interested in the ways you can configure a running `go-sbot`, see the [configuration guide](./docs/config.md).

## Bootstrapping from an existing key-pair

If you have an existing feed with published `contact` messages, you can just resync it from another go or js server. To get this going you copy the key-pair (`$HOME/.ssb/secret` by default) to `$HOME/.ssb-go/secret`, start the program and connect to the server (using the [multiserver address format](https://github.com/ssbc/multiserver/#address-format)).

```bash
mkdir $HOME/.ssb-go
cp $HOME/.ssb/secret $HOME/.ssb-go
go-sbot &
sbotcli connect "net:some.ho.st:8008~shs:SomeActuallyValidPubKey="
```

## Publishing

This currently constructs _legacy_ SSB messages, that _still_ have the signature inside the signed value:

```json
{
  "key": "%EMr6LTquV6Y8qkSaQ96ncL6oymbx4IddLdQKVGqYgGI=.sha256",
  "value": {
    "previous": "%rkJMoEspdU75c1RpGbwjEH7eZxM/PJPFubpZTtynhsg=.sha256",
    "author": "@iL6NzQoOLFP18pCpprkbY80DMtiG4JFFtVSVUaoGsOQ=.ed25519",
    "sequence": 793,
    "timestamp": 1457694632215,
    "hash": "sha256",
    "content": {
      "type": "post",
      "text": "@dust \n> this feels like the cultural opposite of self-dogfooding, and naturally, leaves a bad taste in my mouth \n \n\"This\" meaning this thread? Or something in particular in this thread? And if this thread or something in it, how so? I don't want to leave a bad taste in your mouth.",
      "root": "%I3yWHMF2kqC7fLZrC8FB+Kuu/6MQZIKzJGIjR3fVv9g=.sha256",
      "branch": "%cNJgO+1R4ci/jgTup4LLACoaKZRtYtsO7BzRCDJh6Gg=.sha256",
      "mentions": [
        {
          "link": "@/02iw6SFEPIHl8nMkYSwcCgRWxiG6VP547Wcp1NW8Bo=.ed25519",
          "name": "dust"
        }
      ],
      "channel": "patchwork-dev"
    },
    "signature": "bbjj+zyNubLNEV+hhUf6Of4KYOlQBavQnvdW9rF2nKqTHQTBiFBnRehfveCft3OGSIIr4VgD4ePICCTlBuTdAg==.sig.ed25519"
  },
  "timestamp": 1550074432723.0059
}
```

The problem with this (for Go and others) is removing the `signature` field from `value` without changing any of the values or field ordering of the object, which is required to compute the exact same bytes that were used for creating the signature. Signing JSON was a bad idea. There is also other problems around this (like producing the same byte/string encoding for floats that v8 produces) and a new, canonical format is badly needed.

What you are free to input is the `content` object, the rest is filled in for you. The author is determined by the keypair used by go-sbot. Multiple identities are supported through the API.

### Over muxrpc

go-sbot also exposes the same async [publish](https://scuttlebot.io/apis/scuttlebot/ssb.html#publish-async) method that ssb-server has. So you can also use it with ssb-client!

### Through Go API

To do this programatically in go, you construct a [margaret.Log](https://godoc.org/go.cryptoscope.co/margaret#Log) using `multilogs.OpenPublishLog` ([godoc](https://godoc.org/github.com/ssbc/go-ssb/multilogs#OpenPublishLog)) that publishes the content portion you `Append()` to it the feed of the keypair.

Example:

```go
package main

import (
	"log"
	"fmt"

	"github.com/ssbc/go-ssb"
	"github.com/ssbc/go-ssb/multilogs"
	"github.com/ssbc/go-ssb/sbot"
)

func main() {
	sbot, err := sbot.New()
	check(err)

	publish, err := multilogs.OpenPublishLog(sbot.ReceiveLog, sbot.UserFeeds, *sbot.KeyPair)
	check(err)

	alice, err := refs.ParseFeedRef("@alicesKeyInActualBase64Bytes.ed25519")
	check(err)

	var someMsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": sbot.KeyPair.ID().Ref(),
			"name":  "my user",
		},
		map[string]interface{}{
			"type":      "contact",
			"contact":   alice.Ref(),
			"following": true,
		},
		map[string]interface{}{
			"type": "post",
			"text": `# hello world!`,
		},
		map[string]interface{}{
			"type":  "about",
			"about": alice.Ref(),
			"name":  "test alice",
		},
	}
	for i, msg := range someMsgs {
		newSeq, err := publish.Append(msg)
		check(fmt.Errorf("failed to publish test message %d: %w", i, err))
		log.Println("new message:", newSeq)
	}

	err = sbot.Close()
	check(err)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
```

### `sbotcli`

Has some commands to publish frequently used messages like `post`, `vote` and `contact`:

```bash
sbotcli publish contact --following '@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519'
sbotcli publish contact --blocking '@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519'
sbotcli publish about --name "cryptix" '@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519'
```

They all support passing multiple `--recps` flags to publish private messages as well:
```bash
sbotcli publish post --recps "@key1" --recps "@key2" "what's up?"
```

For more dynamic use, you can also just pipe JSON into stdin:
```bash
cat some.json | sbotcli publish raw
```

## Building

There are two binary executable in this project that are useful right now, both located in the `cmd` folder. `go-sbot` is the database server, handling incoming connections and supplying replication to other peers. `sbotcli` is a command line interface to query feeds and instruct actions like _connect to X_. This also works against the JS implementation.

If you _just_ want to build the server and play without contributing to the code (and are using a recent go version > 1.17), you can do this:

```bash
# clone the repo
git clone https://github.com/ssbc/go-ssb
# go into the servers folder
cd go-ssb/cmd/go-sbot
# build the binary (also fetches pinned dependencies)
go build -v -i
# test the executable works by printing it's help listing
./go-sbot -h
# (optional) install it somwhere on your $PATH
sudo cp go-sbot /usr/local/bin
```

If you want to hack on the other dependencies of the stack, you can use the `replace` statement.

E.g. for hacking on a locally cloned copy of `go-muxrpc`, you'd do:

```diff
diff --git a/go.mod b/go.mod
index c4d475f..51c2757 100644
--- a/go.mod
+++ b/go.mod
@@ -6,6 +6,8 @@ module github.com/ssbc/go-ssb

+replace github.com/ssbc/go-muxrpc/v2 => ./../go-muxrpc
+
```

Which points the new build of `go-sbot`/`sbotcli` to use your local copy of `go-muxrpc`.

## Testing

Once you have configured your environment set up to build the binaries, you can also run the tests. We have unit tests for most of the modules, most importantly `message`, `blobstore` and the replication plugins (`gossip` and `blobs`). There are also interoperability tests with the nodejs implementation (this requires recent versions of [node and npm](http://nodejs.org)).

```bash
$ go test -v ./message
2019/01/08 12:21:55 loaded 236 messages from testdata.zip
=== RUN   TestPreserveOrder
--- PASS: TestPreserveOrder (0.00s)
=== RUN   TestComparePreserve
--- PASS: TestComparePreserve (0.02s)
=== RUN   TestExtractSignature
--- PASS: TestExtractSignature (0.00s)
=== RUN   TestStripSignature
--- PASS: TestStripSignature (0.00s)
=== RUN   TestUnicodeFind
--- PASS: TestUnicodeFind (0.00s)
=== RUN   TestInternalV8String
--- PASS: TestInternalV8String (0.00s)
=== RUN   TestSignatureVerify
--- PASS: TestSignatureVerify (0.06s)
=== RUN   TestVerify
--- PASS: TestVerify (0.06s)
=== RUN   TestVerifyBugs
--- PASS: TestVerifyBugs (0.00s)
PASS
ok  	github.com/ssbc/go-ssb/message	0.180s
```

If you encounter a feed that can't be validated with our code, there is a `encode_test.js` script to create the `testdata.zip` from a local sbot. Call it like this  `cd message && node encode_test.js @feedPubKey.ed25519` and re-run `go test`.

```bash
$ go test ./plugins/...
ok  	github.com/ssbc/go-ssb/plugins/blobs	0.021s
?   	github.com/ssbc/go-ssb/plugins/control	[no test files]
ok  	github.com/ssbc/go-ssb/plugins/gossip	0.667s
?   	github.com/ssbc/go-ssb/plugins/test	[no test files]
?   	github.com/ssbc/go-ssb/plugins/whoami	[no test files]
```

(Sometimes the gossip test blocks indefinitely. This is a bug in go-muxrpcs closing behavior. See [the FAQ](./docs/faq.md) for more information.)

To run the interop tests you need to install the dependencies first and then run the tests. Diagnosing a failure might require adding the `-v` flag to get the stderr output from the nodejs process.

```bash
$ cd message/legacy && npm ci
$ go test -v
$ cd -
$ cd tests && npm ci
$ go test -v
```

## CI

The tests run under [this Github Actions configuration](./.github/workflows/go.yml).

We cache both the global `~/.npm` and `**/node_modules` to reduce build times.
This seems like a reasonable approach because we're testing against a single
Node.js version and a locked package set. Go dependencies are also cached.

## Stack links

* [secret-handshake](https://secret-handshake.club) key exchange using [secretstream](https://godoc.org/github.com/ssbc/go-secretstream)
* JS interoparability by using [go-muxprc](https://godoc.org/github.com/ssbc/go-muxrpc)
* Embedded datastore, no external database required ([margaret](https://godoc.org/github.com/ssbc/margaret) abstraction with [BadgerDB](https://github.com/dgraph-io/badger) backend, similar to [flumedb](https://github.com/flumedb/flumedb))
* [pull-stream](https://pull-stream.github.io)-like abstraction (called [luigi](https://godoc.org/github.com/ssbc/go-luigi)) to pipe between rpc and database.

## Contact

* Post to the `#go-ssb` / `#go-ssb-dev` channels on ssb
* [Raise an issue](https://github.com/ssbc/go-ssb/issues)
* Or mention us individually on ssb
  * cryptix: `@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519`
  * keks: `@YXkE3TikkY4GFMX3lzXUllRkNTbj5E+604AkaO1xbz8=.ed25519`
  * decentral1se `@i8OXtTYaK0PrF002pd4vpXmrlg98As7ZMaHGKoXixdM=.ed25519`
