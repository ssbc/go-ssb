# FAQ

## Is `go-ssb` compatible with other data stores (ssb-db, ssb-db2)?

No, you can't use `go-ssb` API bindings to read / write other ssb client data
stores as of October 2022. You can however use `sbotcli` to query muxrpc
endpoints with clients like Patchwork.

See [`#80`](https://github.com/ssbc/go-ssb/issues/80) for more.

## What platforms does `go-ssb` support?

We've seen reports of butts running `go-ssb` successfully on:

- GNU/Linux
- Mac OS X
- Rasperry Pi 3/4
- Orange Pi Zero
- iOS

Go [supports several other OS/arch](https://go.dev/doc/install/source#environment) possibilities.

Please let us know if you manage to run `go-ssb` on a new platform that is not listed above.
