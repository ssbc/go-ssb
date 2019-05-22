#! /usr/bin/env bash

set -e 

unset GOOS
go install -v -race

export CGO_ENABLED=0
export GOROOT=$HOME/go.root


#export GOOS=linux
#go install -v
#go build -v -i
#zstd go-sbot
#scp go-sbot.zst keksvps:.
#scp go-sbot.zst versepub:.
#rm go-sbot.zst

export GOOS=freebsd
go build -v -i
zstd go-sbot
scp go-sbot aufdie12:.
# todo: restart
scp go-sbot.zst vmbox:.
ssh aufdie12 ./bin/restartGobot.sh
rm go-sbot.zst
