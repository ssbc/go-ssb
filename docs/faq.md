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
