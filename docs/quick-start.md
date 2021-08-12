<!--
SPDX-FileCopyrightText: 2021 The Go-SSB Authors

SPDX-License-Identifier: MIT
-->

# Quick start for go-sbot

This is a quick-start guide to getting go-sbot running on your server and testing that it's working. 

## Install go

Install Go (Version 1.13 or higher) for your operating system: https://golang.org/doc/install

Run, 
`go --version`
to confirm go is installed.

## Install go-ssb

On your server:
```
git clone https://github.com/cryptoscope/ssb
cd ssb
go install ./cmd/go-sbot
go install ./cmd/sbotcli
```

Then make sure that the go binaries are in your PATH. 

You may need to add this to your ~/.bash_profile
```
export PATH=$PATH:${HOME}/go/bin
```

## Run go-sbot 

On the server, run:
```
go-sbot 
```

## Test sbotcli 

While go-sbot is running, in another terminal on the server run:
```
sbotcli call whoami 
```

It should output your ssb public key. 


## Create an invite

The following command creates an invite with 100 uses (number can be changed):
```
sbotcli invite create --uses 100
```

Take the output, and replace [::] with the IP address or domain name pointing to the server. 


## Use the invite 

On your laptop, in your SSB client (Patchwork, Oasis, etc.), redeem the invite. 


## Test everything is working 

On the server run, 
```
sbotcli publish post "your message"
```

You should see the message soon appear in your SSB client. 


## Further commands for sbotcli 

The full documentation for the API for sbotcli is [here (does this exist?)]()

Here are some helpful commands:

See log of all messages (large output):
```
sbotcli log 
```

Follow another SSB server and connect with it. (note you need to follow a pub before you can connect with it)
```
sbotcli publish contact --following @uMiN0TRVMGVNTQUb6KCbiOi/8UQYcyojiA83rCghxGo=.ed25519
sbotcli connect "net:ssb.learningsocieties.org:8008~shs:uMiN0TRVMGVNTQUb6KCbiOi/8UQYcyojiA83rCghxGo="
```

The above commands are to connect to ssb.learningsocieties.org, and the domain and public key can be changed as needed. 


## Permanently run go-sbot 

This could be done in a number of ways. One way to do this:

Create a file called run-go-sbot.sh with the following:
```bash
#!/bin/bash
while true; do
  go-sbot 
  sleep 3
done
```

Then run `sh run-go-sbot.sh` in a detachable session. 


## Troubleshooting 

If your ssb client is unable to redeem the invite created by go-sbot, 
the necessary ports for go-sbot may be blocked on the server. 

For your cloud provider, ensure that port 8008 is open for your server. 


## Further Reading

- https://dev.scuttlebutt.nz/#/golang/
- https://scuttlebutt.nz/




