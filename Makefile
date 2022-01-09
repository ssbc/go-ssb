# SPDX-FileCopyrightText: 2021 The Go-SSB Authors
# SPDX-License-Identifier: MIT

PKGS := $(shell go list ./... | grep -v node_modules )


TESTFLAGS = -failfast -timeout 5m


ZIPPER := zstd -9
ifeq (, $(shell which zstd))
ZIPPER := gzip
endif

# echo -en "\n        $(pkg)\r";
# this echo is just a trick to print the packge that is tested 
# and then returning the carriage to let the go test invocation print the result over the line
# so purly developer experience

.PHONY: test
test:
	$(foreach pkg, $(PKGS), echo -en "\n        $(pkg)\r"; LIBRARIAN_WRITEALL=0 go test $(TESTFLAGS) $(pkg) || exit 1;)

.PHONY: racetest
racetest:
	$(foreach pkg, $(PKGS), echo -en "\n        $(pkg)\r"; LIBRARIAN_WRITEALL=0 go test $(TESTFLAGS) -race $(pkg) || exit 1;)

VERSION = $(shell git describe --tags --exact-match)
ifeq ($(VERSION),)
	VERSION = $(shell git rev-parse --short HEAD)
endif
BUILD=$(shell date +%FT%T%z)

LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION} -X main.Build=${BUILD}"

PLATFORMS := windows-amd64 windows-arm64 linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 freebsd-amd64
os = $(word 1,$(subst -, ,$@))
arch = $(word 2,$(subst -, ,$@))

ARCHIVENAME=release/$(os)-$(arch)-$(VERSION).tar

.PHONY: $(PLATFORMS)

$(PLATFORMS):
	GOOS=$(os) GOARCH=$(arch) go build -v -i -trimpath $(LDFLAGS) -o go-sbot ./cmd/go-sbot
	GOOS=$(os) GOARCH=$(arch) go build -v -i -trimpath $(LDFLAGS) -o sbotcli ./cmd/sbotcli
	GOOS=$(os) GOARCH=$(arch) go build -v -i -trimpath $(LDFLAGS) -o gossb-truncate-log ./cmd/ssb-truncate-log
	GOOS=$(os) GOARCH=$(arch) go build -v -i -trimpath $(LDFLAGS) -o ssb-offset-converter ./cmd/ssb-offset-converter
	tar cvf $(ARCHIVENAME) go-sbot sbotcli gossb-truncate-log ssb-offset-converter 
	rm go-sbot sbotcli gossb-truncate-log ssb-offset-converter
	$(ZIPPER) $(ARCHIVENAME)
	rm $(ARCHIVENAME) || true

.PHONY: release
release: windows-amd64 windows-arm64 linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 freebsd-amd64
