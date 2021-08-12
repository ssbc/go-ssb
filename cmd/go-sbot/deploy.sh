#! /usr/bin/env bash

# SPDX-FileCopyrightText: 2021 The Go-SSB Authors
#
# SPDX-License-Identifier: MIT

set -e 

unset GOOS
go install -v

export CGO_ENABLED=0
#export GOROOT=$HOME/go.root


export GOOS=linux
go build -v -i
zstd go-sbot
scp go-sbot.zst keksvps:.
#scp go-sbot.zst versepub:.
rm go-sbot.zst

export GOOS=freebsd
go build -v -i
zstd go-sbot
scp go-sbot.zst vmbox:.
# todo: restart
gzip go-sbot
scp go-sbot.gz aufdie12:.
ssh aufdie12 ./bin/restartGobot.sh
rm go-sbot.zst
rm go-sbot.gz
