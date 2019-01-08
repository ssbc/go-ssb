# Go-SSB [![GoDoc](https://godoc.org/go.cryptoscope.co/ssb?status.svg)](https://godoc.org/go.cryptoscope.co/ssb)

This is a work-in-progress full-stack implementaion of [secure-scuttlebutt](https://www.scuttlebutt.nz) using the [Go](https://golang.org) programming language.

## Server Features

* [x] Follow-graph implementation (based on [gonum](https://www.gonum.org)) to authorize incomming connections
* [x] [Blobs](https://ssbc.github.io/scuttlebutt-protocol-guide/#blobs) store and replication
* [x] _Legacy_ gossip [replication](https://ssbc.github.io/scuttlebutt-protocol-guide/#createHistoryStream) ([ebt](https://github.com/dominictarr/epidemic-broadcast-trees) not implementation yet)
* [ ] Publishing new messages to the log
* [ ] Invite mechanics (might wait for direct-user-invites to stabalize)

## Building

We are trying to adopt the new [Go Modules](https://github.com/golang/go/wiki/Modules) way of defining dependencies and therefore require at least Go version 1.11 to build with the `go.mod` file definitions. (Building with erlier versions still possible, though. We keep an intact dependency tree in `vendor/`, populated by `go mod vendor`, which is picked up by default since Go 1.09.)

There are two binary exeutables in this repo that are usefull right now, both located in the `cmd` folder. `go-sbot` is the database server, handling incoming connections and supplying replication to other peers. `sbotcli` is a command line interface to query feeds and instruct actions like _connect to X_. This also works against the JS implementation.

If you _just_ want to build the server and play without without contributing to the code (and are using a recent go version > 1.11), you can do this:

```bash
# clone the repo
git clone https://github.com/cryptoscope/ssb
# go into the servers folder
cd ssb/cmd/go-sbot
# build the binary (also fetches pinned dependencies)
go build -v -i
# test the executable works by printing it's help listing
./go-sbot -h
# (optional) install it somwhere on your $PATH
sudo cp go-sbot /usr/local/bin
```

If you want to hack on the other dependencies of the stack, we still advise to use the classic Go way with a `$GOPATH`. This way you have all the code available to inspect and change. (Go modules get stored in a read-only cache. Replacing them needs a checkout on an individual basis.)

```bash
# prepare workspace for all the go code
export GOPATH=$HOME/proj/go-ssb
mkdir -p $GOPATH
# fetch project source and dependencies
go get -v -u go.cryptoscope.co/ssb
# change to the project directory
cd $GOPATH/src/go.cryptoscope.co/ssb
# build the binaries (will get saved to $GOPATH/bin)
go install ./cmd/go-sbot
go install ./cmd/sbotcli
```

## Testing [![Build Status](https://travis-ci.org/cryptoscope/ssb.svg?branch=master)](https://travis-ci.org/cryptoscope/ssb)

Once you have configured your environment set up to build the binaries, you can also run the tests. We have unit tests for most of the modules, most importantly `message`, `blobstore` and the replication plugins (`gossip` and `blobs`). There are also interoparability tests with the nodejs implementation (this requires recent versions of [node and npm](http://nodejs.org)).

```bash
$ cd $GOPATH/src/go.cryptoscope.co/ssb

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
ok  	go.cryptoscope.co/ssb/message	0.180s
```

If you encounter a feed that can't be validated with our code, there is a `encode_test.js` script to create the `testdata.zip` from a local sbot. Call it like this  `cd message && node encode_test.js @feedPubKey.ed25519` and re-run `go test`.

```bash
$ go test ./plugins/...
ok  	go.cryptoscope.co/ssb/plugins/blobs	0.021s
?   	go.cryptoscope.co/ssb/plugins/control	[no test files]
ok  	go.cryptoscope.co/ssb/plugins/gossip	0.667s
?   	go.cryptoscope.co/ssb/plugins/test	[no test files]
?   	go.cryptoscope.co/ssb/plugins/whoami	[no test files]
```

(Sometimes the gossip test blocks indefinitly. This is a bug in go-muxrpcs closing behavior. See the _Known bugs_ section for more information.)


To run the interop tests you need to install the dependencies first and then run the tests. Diagnosing a failure might require adding the `-v` flag to get the stderr output from the nodejs process.

```bash
$ cd $GOPATH/src/go.cryptoscope.co/ssb/tests
$ npm ci
$ go test -v
```

## Bootstrapping from an existing key-pair

Until we implemented _publish_, the way to use our pub implementation requires an existing feed with published `type:contact` messages. To get this going you copy the key-pair (`$HOME/.ssb/secret` by default) to `$HOME/.ssb-go/secret` and start the program.

```bash
mkdir $HOME/.ssb-go
cp $HOME/.ssb/secret $HOME/.ssb-go
go-sbot &
sbotcli connect "
```

## Known Bugs

### compliation error regarding Badger

Badger pushed an API change to master. We still depend on v1.5.4 as there is only a candidate release versioned of the new API yet.

```
# go.cryptoscope.co/librarian/badger
./index.go:53:13: assignment mismatch: 2 variables but 1 values
./index.go:53:26: not enough arguments in call to item.Value
	have ()
	want (func([]byte) error)
```

Either use the _Go Module_ way of building the project, which uses the pinned version specified by the `go.mod` file or check out the specific version of badger in your `$GOPATH`.


### Startup error / illegal JSON value

We currently use a [very rough state file](https://github.com/keks/persist) to keep track of which messages are indexed already (multilogs and contact graph). When the server crashes while it is being rewritten, this file can get corrupted. We have a fsck-like tool in mind to rebuild the indicies from the static log but it's not done yet.

```
TODO: example error
```

Our current workaround is to do a full resync from the network:

```bash
kill $(pgrep go-sbot)
rm -rf $HOME/.ssb-go/{log,sublogs,indicies}
go-sbot &
sbotcli connect "host:port:@pubkey"
```

### muxrpc closing

There is race in our muxrpc port that sometimes leaves querys hanging and thus doesn't free memory or TCP connections. TODO: issue link

## Stack links

* [secret-handshake](https://secret-handshake.club) key exchange using [secretstream](https://godoc.org/go.cryptoscope.co/secretstream)
* JS interoparability by using [go-muxprc](https://godoc.org/go.cryptoscope.co/muxrpc)
* Embedded datastore, no external database required ([librarian](https://godoc.org/go.cryptoscope.co/librarian) abstraction with [BadgerDB](https://github.com/dgraph-io/badger) backend, similar to [flumedb](https://github.com/flumedb/flumedb))
* [pull-stream](https://pull-stream.github.io)-like abstraction (called [luigi](https://godoc.org/go.cryptoscope.co/luigi)) to pipe between rpc and database.


## Contact

Either post to the #go-ssb channel on the mainnet or mention us individually:

* cryptix: `@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519`
* keks: `@YXkE3TikkY4GFMX3lzXUllRkNTbj5E+604AkaO1xbz8=.ed2551`