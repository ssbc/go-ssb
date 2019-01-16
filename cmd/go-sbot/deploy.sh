#! /usr/bin/env bash

set -e 
export CGO_ENABLED=0
#export GOROOT=$HOME/go.tmproot

export GOOS=linux
go install -v
go build -v -i
gzip go-sbot
scp go-sbot.gz keksvps:.
rm go-sbot.gz

export GOOS=freebsd
go build -v -i
gzip go-sbot
scp go-sbot.gz aufdie12:.
# todo: restart
scp go-sbot.gz vmbox:.
# todo: restart
rm go-sbot.gz
