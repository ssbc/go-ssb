# FAQ

## Is `go-ssb` compatible with other data stores (ssb-db, ssb-db2)?

No, you can't use `go-ssb` API bindings to read / write other ssb client data
stores as of October 2022. You can however use `sbotcli` to query muxrpc
endpoints with clients like Patchwork.

See [`#80`](https://github.com/ssbc/go-ssb/issues/80) for more.
