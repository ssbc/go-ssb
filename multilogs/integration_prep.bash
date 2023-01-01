#!/usr/bin/env bash

# SPDX-FileCopyrightText: 2021 The Go-SSB Authors
#
# SPDX-License-Identifier: MIT

set -x

dest=$1
test "$dest" != "" || {
    echo "dest: ${dest} not set"
    exit 1
}


sha256sum -c v3-sloop-m100000-a2000.tar.gz.shasum || {
    wget "https://github.com/ssbc/ssb-fixtures/releases/download/3.0.2/v3-sloop-m100000-a2000.tar.gz"

    sha256sum -c v3-sloop-m100000-a2000.tar.gz.shasum || {
        echo 'download of ssb-fixtures failed'
        exit 1
    }
}


rm -r tmp
rm -r testrun

mkdir -p tmp/unpack
tar xf v3-sloop-m100000-a2000.tar.gz -C tmp/unpack

go run ../cmd/ssb-offset-converter -if lfo tmp/unpack/flume/log.offset $dest
