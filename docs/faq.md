<!--
SPDX-FileCopyrightText: 2023 The Go-SSB Authors

SPDX-License-Identifier: MIT
-->

# FAQ

## Is `go-ssb` production ready?

No, not yet. It is still in the alpha stages and backwards incompatible changes will be made.

## Is `go-ssb` compatible with other data stores (ssb-db, ssb-db2)?

No, you can't use `go-ssb` API bindings to read / write other ssb client data
stores as of October 2022. You can however use `sbotcli` to query muxrpc
endpoints with clients like Patchwork.

See [`#80`](https://github.com/ssbc/go-ssb/issues/80) for more.

## Can `go-ssb` replicate with Manyverse?

Not reliably, as EBT replication still has some issues (see below). What you can do though is replicate with other Patchwork users. And Patchwork can replicate with Manyverse. So you just need one Patchwork user in your network and gossip should work.

## What platforms does `go-ssb` support?

> **WARNING**: we've seen reports of data loss issues on 32 bit architectures, e.g. some of the Orange Pi Zero earlier series and older Raspberry Pis. This is being investigated on [`go-ssb/#183`](https://github.com/ssbc/go-ssb/issues/183). We would recommend avoiding 32 bit architecture systems until this issue has been wrapped up.

We've seen reports of butts running `go-ssb` successfully on:

- GNU/Linux
- Mac OS X
- Raspberry Pi 3/4 (amd64/arm64)
- Orange Pi Zero (amd64/arm64)
- iOS

Go [supports several other OS/arch](https://go.dev/doc/install/source#environment) possibilities.

Please let us know if you manage to run `go-ssb` on a new platform that is not listed above.

## Does EBT (`-enable-ebt`) replication work?

We've seen several reports (e.g. `%lmBRs0eSQP9JLTuVgVEipf4a9Ke5uSHZq0xt8UIzQSs=.sha256`) that it does not work reliably in the current state (November 2022). There are a number of [EBT related fixes living on the Planetary fork](https://github.com/planetary-social/ssb/tree/fork) which might help. Those patches were / are being run by Planetary and others [have tested them](https://github.com/ssbc/go-ssb/pull/180#issuecomment-1295784977), YMMV!

## What is the relationship between `go-ssb` & `scuttlego`?

It's still a bit fuzzy but pulling some notes from the previous discussions: `scuttlego` is a new effort to build "an embeddable sdk for building lots of ssb apps, not just for planetary" which is being developed by Go hackers @ [Planetary](https://planetary.social). It is still in the early stages of development. For more background information, see `%AfPRFA+lc8bu95GQ04q43prH65jDukkSn8cBIg6Lbyc=.sha256` and [`github.com/planetary-social/scuttlego`](https://github.com/planetary-social/scuttlego).

## What are *legacy messages*?

See [`ssbc/go-ssb#87`](https://github.com/ssbc/go-ssb/issues/87) for more.

## What is *legacy replication*?

This means the way clients send / request messages from other peers. The SSB
software ecosystem is moving and work is being done to make exchanging messages
more performant.

The new approach is called "EBT replication" and you can learn more about that
on [this repository](https://github.com/ssbc/ssb-ebt).

`go-ssb` supports both *legacy replication* (pre-EBT replication) and EBT
replication (although it is slightly buggy as of November 2022, see above for
more).

## `go-ssb` is not initiating outgoing connections?

The implementation currently does not include an outgoing connection scheduler. This means
that `go-ssb` will only replicate when it gets connected to or you initiate a conneciton
via `sbotcli`.

You can work around this by setting up a script like the one below. The
multiserver addresses included for documentation purposes, please replace them
with your own.

```bash
cat > /tmp/connect.hosts << EOF
net:cryptbox.mindeco.de:8008~shs:xxZeuBUKIXYZZ7y+CIHUuy0NOFXz2MhUBkHemr86S3M=
net:cryptoscope.co:8008~shs:G20wv2IZkB7BA/LKe7WMuByop3++J0u9+Y32OoEVOj8=
net:one.butt.nz:8008~shs:VJM7w1W19ZsKmG2KnfaoKIM66BRoreEkzaVm/J//wl8=
net:pub.t4l3.net:8008~shs:WndnBREUvtFVF14XYEq01icpt91753bA+nVycEJIAX4=
EOF

cat /tmp/connect.hosts | while read maddr
do
  sbotcli connect $maddr
  sleep 600
done
```

See [`ssbc/go-ssb#66`](https://github.com/ssbc/go-ssb/issues/66) for more.

## Can I use `sbotc` to query `go-ssb`?

Some commands are supported but not all. The safest bet is to use `sbotcli`
packaged in this repository also. Client compatibility is desired but we may
not have the capacity to handle all cases. Please raise [an
issue](https://github.com/ssbc/go-ssb/issues) if you see a missing MUXRPC
endpoint.

Please see [`ssbc/go-ssb/#62`](https://github.com/ssbc/go-ssb/issues/62) for more.

### Forked version of x/crypto

We currently depend on [this patch](https://github.com/cryptix/golang_x_crypto/tree/non-internal-edwards) on x/crypto to support the key-material conversion between ed25519 and curve25519.  See https://github.com/ssbc/go-ssb/issues/44 for all the details.

```
package golang.org/x/crypto/ed25519/edwards25519: cannot find package "golang.org/x/crypto/ed25519/edwards25519" in any of:
	/home/cryptix/go.root/src/golang.org/x/crypto/ed25519/edwards25519 (from $GOROOT)
	/home/cryptix/go-fooooo/src/golang.org/x/crypto/ed25519/edwards25519 (from $GOPATH)
```

If you see the above error, make sure your project has the following replace directive in place:

```
replace golang.org/x/crypto => github.com/cryptix/golang_x_crypto v0.0.0-20200303113948-2939d6771b24
```

### Startup error / illegal JSON value

We currently use a [very rough state file](https://github.com/keks/persist) to keep track of which messages are indexed already (multilogs and contact graph). When the server crashes while it is being rewritten, this file can get corrupted. We have a fsck-like tool in mind to rebuild the indicies from the static log but it's not done yet.

```
time=2019-01-09T21:19:08.73736113Z caller=new.go:47 module=sbot event="component terminated" component=userFeeds error="error querying rootLog for mlog: error decoding value: json: cannot unmarshal number 272954244983260621261341 into Go value of type margaret.Seq"
```

Our current workaround is to do a full resync from the network:

```bash
kill $(pgrep go-sbot)
rm -rf $HOME/.ssb-go/{log,sublogs,indicies}
go-sbot &
sbotcli connect "net:some.ho.st:8008~shs:SomeActuallyValidPubKey="
```

### `go-ssb` is too memory hungry?

Try building with the `-tags lite` build tag:

```
go build -tags lite ./cmd/go-sbot
```

It uses a lightweight BadgerDB configuration that [Planetary](https://github.com/planetary-social/ssb/blob/a76247b9e67a2792113f33840d1f15bbb1467d93/repo/badger_ios.go) have been running.

Try also experimenting with `-numPeer` / `-numRepl` if you're using legacy gossip replication. This will limit the amount of replication that can happen concurrently. See [`#124`](https://github.com/ssbc/go-ssb/issues/124) for more details.
