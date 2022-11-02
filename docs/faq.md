# FAQ

## Is `go-ssb` compatible with other data stores (ssb-db, ssb-db2)?

No, you can't use `go-ssb` API bindings to read / write other ssb client data
stores as of October 2022. You can however use `sbotcli` to query muxrpc
endpoints with clients like Patchwork.

See [`#80`](https://github.com/ssbc/go-ssb/issues/80) for more.

## What platforms does `go-ssb` support?

> **WARNING**: we've seen reports of data loss issues on 32 bit architectures, e.g. some of the Orage Pi Zero earlier series and older Rasperry Pis. This is being investigated on [`go-ssb/#183`](https://github.com/ssbc/go-ssb/issues/183). We would recommend avoiding 32 bit architecture systems until this issue has been wrapped up.

We've seen reports of butts running `go-ssb` successfully on:

- GNU/Linux
- Mac OS X
- Rasperry Pi 3/4 (amd64/arm64)
- Orange Pi Zero (amd64/arm64)
- iOS

Go [supports several other OS/arch](https://go.dev/doc/install/source#environment) possibilities.

Please let us know if you manage to run `go-ssb` on a new platform that is not listed above.
