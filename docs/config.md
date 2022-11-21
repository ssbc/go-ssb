# Configuration

go-ssb's `go-sbot` can be configured in three different ways:

1. Using the `--flags` parameters, run `./go-sbot --help` to see a list of possible flags
1. Using environment variables
1. Using a configuration file, written in [toml](https://en.wikipedia.org/wiki/TOML)

This little guide deals with detailing the last two options.

In the underlying implementation, there's an order that determines which value a particular `go-sbot`
setting takes on if it's presented with a mix of flags, environment variables, or a config.

The precedence order goes as follows:

* flag-set values trump environment variables
* environment variables trump config values
* config-set values trump default flag values
* default flag values are the final fallback, if the corresponding config value or environment variable has not been set

## Configuration file
The default location for the config file is `~/.ssb-go/config.toml`. The order of precedence
when it comes to loading a config file is as follows:

* 1. Environment variable `$SSB_CONFIG_FILE` or flag `--config` are used first
* 2. Lacking that, the location defined by `--repo` is used
* 3. The final fallback is to the default location at ~/.ssb-go/config.toml


Below you may find a complete example of the config file, any values you comment out or leave
as blanks `""` will be ignored.

**Note** the keys of the config file are the same as the names of the equivalent flags which
can be used when invoking `go-sbot`. For example, if you wanted to set the `hops` to 2 and the
default ssb-go location to somewhere else using only flags, that would look like:

```
./go-sbot -hops 2 -repo /var/scuttlebutt
```

### Example config

```toml
# Where to put the log and indexes
repo = '.ssb-go'
# Where to write debug output: NOTE, this is relative to "repo" atm
debugdir = ''

# Secret-handshake app-key (or compatible alt-key)
shscap = "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s="
# If set, sign with hmac hash of msg instead of plain message object using this key
hmac = ""
# How many hops to fetch (1: friends, 2: friends of friends); note that a nodejs hops value needs to be decreased by one in go-sbot
# e.g. go-sbot hops of 1 <=> ssb-js hops of 2
hops = 1

# how many feeds can be replicated with one peer connection using legacy gossip (shouldn't be higher than numRepl)
numPeer = 5
# how many feeds can be replicated concurrently using legacy gossip
numRepl = 10

# Address to listen on
lis = ":8008"
# Address to listen on for ssb websocket connections
wslis = ":8989"
# Address to listen on for metrics and pprof HTTP server
debuglis = "localhost:6078"

# Enable sending local UDP broadcasts
localadv = false
# Enable connecting to incoming UDP broadcasts
localdiscov = false
# Enable syncing by using epidemic-broadcast-trees (EBT)
enable-ebt = false
# Bypass graph auth and fetch remote's feed, useful for pubs that are restoring their data from peers. Caveats abound, however.
promisc = false
# Disable the UNIX socket RPC interface
nounixsock = false
```

## Environment Variables
Environment variables are a common way of customizing options of various services. For go-sbot, that could for instance take the appearance of

```
SSB_CONFIG_FILE="~/.ssb-go-config.toml" ./go-sbot
```

### Environment variable listing
```sh
SSB_DATA_DIR="/var/lib/ssb-server"
SSB_CONFIG_FILE="/etc/ssb-server/config"
SSB_LOG_DIR="/var/log/ssb-server"

SSB_CAP_SHS_KEY=""
SSB_CAP_HMAC_KEY=""
SSB_HOPS=2

SSB_MUXRPC_ADDRESS=":8008"
SSB_WS_ADDRESS=":8989"
SSB_PROMETHEUS_ADDRESS="localhost:6078"  // aka debug metrics

SSB_PROMETHEUS_ENABLED=no
SSB_EBT_ENABLED=no
SSB_CONN_FIREWALL_ENABLED=yes // equivalent with --promisc
SSB_CONN_DISCOVERY_UDP_ENABLED=no
SSB_CONN_BROADCAST_UDP_ENABLED=no

// limited replication
SSB_NUM_PEER=5
SSB_NUM_REPL=10

// go-ssb specific (for peachpub compat purposes)
GO_SSB_REPAIR_FS=no

// SSB_LOG_LEVEL="info" currently not implemented
// SSB_CAP_INVITE_KEY="" currently not implemented
// SSB_SOCKET_ENABLED=no currently not implemented
```

## Inspecting configured values

If your use case necessitates grabbing the running sbot's currently configured values, whether
be they from the configuration file or from environment variables, then these are exposed in a
particular file called `running-config.json`.

This file is located in relation to the configuration directory specified by the --config flag,
and defaults to `~/.ssb-go/running-config.json`.

**Note**: overrides from --flag options will not be represented in `running-config.json`.
